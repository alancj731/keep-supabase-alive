package com.jian.supabasekeepalive.service;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.atomic.AtomicReference;

import com.jian.supabasekeepalive.config.KeepaliveProperties;
import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.RunSummary;
import com.jian.supabasekeepalive.model.SupabaseProject;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Runs {@code select * from <table> limit 1} against every configured project. The query result is
 * irrelevant — the point is the database activity, which is what stops Supabase pausing a free-tier
 * project. The latest outcome per project is kept in memory for /api/keepalive/status and health.
 */
public class KeepaliveService {

    private static final Logger logger = LoggerFactory.getLogger(KeepaliveService.class);

    private final ProjectRegistry registry;

    private final JdbcConnectionFactory connectionFactory;

    private final KeepaliveProperties properties;

    private final Map<String, PingResult> lastResults = new ConcurrentHashMap<>();

    private final AtomicReference<RunSummary> lastRun = new AtomicReference<>();

    private final Object runGuard = new Object();

    private final Duration staleRunAfter;

    private boolean running;

    private Instant activeFrom;

    private long generation;

    public KeepaliveService(ProjectRegistry registry, JdbcConnectionFactory connectionFactory,
            KeepaliveProperties properties) {
        this(registry, connectionFactory, properties,
                Duration.ofSeconds(Math.max(1, properties.getStaleRunAfterSeconds())));
    }

    KeepaliveService(ProjectRegistry registry, JdbcConnectionFactory connectionFactory,
            KeepaliveProperties properties, Duration staleRunAfter) {
        this.registry = registry;
        this.connectionFactory = connectionFactory;
        this.properties = properties;
        this.staleRunAfter = staleRunAfter.isNegative() || staleRunAfter.isZero()
                ? Duration.ofMinutes(5) : staleRunAfter;
    }

    /**
     * Pings every project, in parallel, and records the outcome.
     *
     * @throws RunInProgressException if another run has not finished yet
     */
    public RunSummary runAll(String trigger) {
        long claimedGeneration = beginRun();
        try {
            List<SupabaseProject> projects = this.registry.projects();
            Instant startedAt = Instant.now();
            List<PingResult> results = new ArrayList<>(projects.size());
            try (ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor()) {
                List<Future<PingResult>> futures = new ArrayList<>(projects.size());
                for (SupabaseProject project : projects) {
                    futures.add(executor.submit(() -> ping(project)));
                }
                // close() (from try-with-resources) waits for every task, so each future is done.
                for (int i = 0; i < futures.size(); i++) {
                    results.add(resultOf(futures.get(i), projects.get(i)));
                }
            }
            RunSummary summary = RunSummary.of(trigger, startedAt, Instant.now(), results);
            this.lastRun.set(summary);
            logger.info("Keep-alive run ({}) finished: {}/{} project(s) OK in {} ms", trigger, summary.succeeded(),
                    summary.total(), summary.durationMs());
            return summary;
        }
        finally {
            endRun(claimedGeneration);
        }
    }

    /**
     * Claims the in-flight guard. Hosted processes can be suspended mid-request, so a guard older
     * than the configured limit is considered abandoned and may be taken over. A superseded run
     * cannot clear the replacement run's guard when it eventually resumes.
     */
    private long beginRun() {
        synchronized (this.runGuard) {
            Instant now = Instant.now();
            if (this.running) {
                Duration held = Duration.between(this.activeFrom, now);
                if (held.compareTo(this.staleRunAfter) < 0) {
                    throw new RunInProgressException();
                }
                logger.warn("Taking over a keep-alive run that never finished after {} (limit {})", held,
                        this.staleRunAfter);
            }
            this.running = true;
            this.activeFrom = now;
            return ++this.generation;
        }
    }

    private void endRun(long claimedGeneration) {
        synchronized (this.runGuard) {
            if (this.generation == claimedGeneration) {
                this.running = false;
                this.activeFrom = null;
            }
        }
    }

    private PingResult resultOf(Future<PingResult> future, SupabaseProject project) {
        try {
            return future.get();
        }
        catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
            return PingResult.failure(project, 0, 0, "interrupted");
        }
        catch (Exception ex) {
            // ping() handles its own failures; this only guards against something truly unexpected.
            logger.error("Keep-alive task for {} ended unexpectedly", project.name(), ex);
            return PingResult.failure(project, 0, 0, ex.getClass().getSimpleName());
        }
    }

    private PingResult ping(SupabaseProject project) {
        String sql = "select * from " + project.quotedTable() + " limit 1";
        int maxAttempts = Math.max(1, this.properties.getRetryAttempts());
        long start = System.nanoTime();
        String lastError = "unknown error";
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            logger.debug("Connecting to {} as {} (attempt {}/{})", project.jdbcUrl(), project.username(), attempt,
                    maxAttempts);
            try (Connection connection = this.connectionFactory.open(project);
                    Statement statement = connection.createStatement()) {
                logger.debug("Connected to {}; running: {}", project.name(), sql);
                statement.setQueryTimeout(this.properties.getQueryTimeoutSeconds());
                int rows = 0;
                try (ResultSet resultSet = statement.executeQuery(sql)) {
                    while (resultSet.next()) {
                        rows++;
                    }
                }
                long durationMs = elapsedMs(start);
                logger.info("Keep-alive OK: {} table {} ({} row(s), {} ms, attempt {}/{})", project.name(),
                        project.table(), rows, durationMs, attempt, maxAttempts);
                return record(PingResult.success(project, attempt, durationMs, rows));
            }
            catch (SQLException | RuntimeException ex) {
                lastError = describe(ex, project.password());
                if (attempt < maxAttempts) {
                    logger.warn("Keep-alive attempt {}/{} failed for {}: {}", attempt, maxAttempts, project.name(),
                            lastError);
                    if (!backoff(attempt)) {
                        break;
                    }
                }
                else {
                    logger.error("Keep-alive FAILED for {} table {} after {} attempt(s): {}", project.name(),
                            project.table(), maxAttempts, lastError);
                }
            }
        }
        return record(PingResult.failure(project, maxAttempts, elapsedMs(start), lastError));
    }

    /** @return false if the wait was interrupted and the retry loop should stop */
    private boolean backoff(int attempt) {
        long waitMs = Math.max(0, this.properties.getRetryBackoffMs()) * attempt;
        if (waitMs == 0) {
            return true;
        }
        try {
            Thread.sleep(waitMs);
            return true;
        }
        catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
            return false;
        }
    }

    /** Builds a log-safe message: SQLState plus text, with the password scrubbed just in case. */
    private static String describe(Exception ex, String password) {
        String message = (ex.getMessage() == null) ? ex.getClass().getSimpleName() : ex.getMessage();
        if (ex instanceof SQLException sqlException && sqlException.getSQLState() != null) {
            message = "[SQLState " + sqlException.getSQLState() + "] " + message;
        }
        // Guard against scrubbing a very short password out of unrelated words.
        if (password != null && password.length() >= 4) {
            message = message.replace(password, "***");
        }
        return message.replaceAll("\\s+", " ").trim();
    }

    private static long elapsedMs(long startNanos) {
        return (System.nanoTime() - startNanos) / 1_000_000L;
    }

    private PingResult record(PingResult result) {
        this.lastResults.put(result.projectId(), result);
        return result;
    }

    /** Latest result per project, in configured order; projects never pinged yet are omitted. */
    public List<PingResult> lastResults() {
        return this.registry.projects().stream().map(project -> this.lastResults.get(project.id()))
                .filter(java.util.Objects::nonNull).toList();
    }

    public Optional<RunSummary> lastRun() {
        return Optional.ofNullable(this.lastRun.get());
    }

    public boolean isRunning() {
        synchronized (this.runGuard) {
            return this.running;
        }
    }

    public int projectCount() {
        return this.registry.size();
    }
}
