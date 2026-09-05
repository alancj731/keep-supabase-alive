package com.jian.supabasekeepalive.service;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import com.jian.supabasekeepalive.config.KeepaliveProperties;
import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.RunSummary;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatExceptionOfType;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class KeepaliveServiceTest {

    private static final String URL = "postgresql://postgres.abc:s3cr3t-password@db.example.com:5432/postgres";

    private final KeepaliveProperties properties = properties();

    private final ProjectRegistry registry = new ProjectRegistry(List.of(URL), List.of("public.users"));

    @Test
    void runsTheLimitOneQueryAgainstTheQuotedTable() throws SQLException {
        Statement statement = statementReturningRows(1);
        KeepaliveService service = new KeepaliveService(this.registry, project -> connectionFor(statement),
                this.properties);

        RunSummary summary = service.runAll("test");

        ArgumentCaptor<String> sql = ArgumentCaptor.forClass(String.class);
        verify(statement).executeQuery(sql.capture());
        assertThat(sql.getValue()).isEqualTo("select * from \"public\".\"users\" limit 1");
        assertThat(summary.succeeded()).isEqualTo(1);
        assertThat(summary.failed()).isZero();
        assertThat(summary.results()).singleElement()
                .satisfies(result -> {
                    assertThat(result.success()).isTrue();
                    assertThat(result.attempts()).isEqualTo(1);
                    assertThat(result.rowsSeen()).isEqualTo(1);
                    assertThat(result.error()).isNull();
                });
        assertThat(service.lastRun()).contains(summary);
        assertThat(service.lastResults()).hasSize(1);
    }

    @Test
    void succeedsOnAnEmptyTable() throws SQLException {
        Statement statement = statementReturningRows(0);
        KeepaliveService service = new KeepaliveService(this.registry, project -> connectionFor(statement),
                this.properties);

        assertThat(service.runAll("test").results()).singleElement()
                .satisfies(result -> assertThat(result.rowsSeen()).isZero());
    }

    @Test
    void retriesAndRecoversWithinTheAttemptBudget() throws SQLException {
        Statement statement = statementReturningRows(1);
        AtomicInteger calls = new AtomicInteger();
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            if (calls.incrementAndGet() == 1) {
                throw new SQLException("connection refused", "08001");
            }
            return connectionFor(statement);
        }, this.properties);

        PingResult result = service.runAll("test").results().getFirst();

        assertThat(result.success()).isTrue();
        assertThat(result.attempts()).isEqualTo(2);
        assertThat(calls).hasValue(2);
    }

    @Test
    void recordsAFailureAfterExhaustingRetries() {
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            throw new SQLException("password authentication failed", "28P01");
        }, this.properties);

        RunSummary summary = service.runAll("test");

        assertThat(summary.failed()).isEqualTo(1);
        assertThat(summary.results()).singleElement().satisfies(result -> {
            assertThat(result.success()).isFalse();
            assertThat(result.attempts()).isEqualTo(3);
            assertThat(result.rowsSeen()).isNull();
            assertThat(result.error()).contains("28P01").contains("password authentication failed");
        });
    }

    @Test
    void keepsThePasswordOutOfTheRecordedError() {
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            throw new SQLException("FATAL: password s3cr3t-password rejected", "28P01");
        }, this.properties);

        assertThat(service.runAll("test").results().getFirst().error()).doesNotContain("s3cr3t-password")
                .contains("***");
    }

    @Test
    void rejectsAConcurrentRun() throws Exception {
        CountDownLatch pingStarted = new CountDownLatch(1);
        CountDownLatch release = new CountDownLatch(1);
        Statement statement = statementReturningRows(1);
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            pingStarted.countDown();
            try {
                release.await(5, TimeUnit.SECONDS);
            }
            catch (InterruptedException ex) {
                Thread.currentThread().interrupt();
            }
            return connectionFor(statement);
        }, this.properties);

        Thread first = new Thread(() -> service.runAll("first"));
        first.start();
        try {
            assertThat(pingStarted.await(5, TimeUnit.SECONDS)).isTrue();
            assertThat(service.isRunning()).isTrue();
            assertThatExceptionOfType(RunInProgressException.class).isThrownBy(() -> service.runAll("second"));
        }
        finally {
            release.countDown();
            first.join(TimeUnit.SECONDS.toMillis(10));
        }
        assertThat(service.isRunning()).isFalse();
    }

    @Test
    void staleRunCanBeTakenOverWithoutClearingTheReplacementGuard() throws Exception {
        CountDownLatch firstStarted = new CountDownLatch(1);
        CountDownLatch secondStarted = new CountDownLatch(1);
        CountDownLatch releaseFirst = new CountDownLatch(1);
        CountDownLatch releaseSecond = new CountDownLatch(1);
        AtomicInteger calls = new AtomicInteger();
        AtomicReference<Throwable> backgroundFailure = new AtomicReference<>();
        Statement statement = statementReturningRows(1);
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            int call = calls.incrementAndGet();
            CountDownLatch started = (call == 1) ? firstStarted : secondStarted;
            CountDownLatch release = (call == 1) ? releaseFirst : releaseSecond;
            started.countDown();
            try {
                release.await(5, TimeUnit.SECONDS);
            }
            catch (InterruptedException ex) {
                Thread.currentThread().interrupt();
            }
            return connectionFor(statement);
        }, this.properties, Duration.ofMillis(10));

        Thread first = new Thread(() -> runCapturing(service, "frozen", backgroundFailure));
        first.start();
        assertThat(firstStarted.await(5, TimeUnit.SECONDS)).isTrue();
        Thread.sleep(30);

        Thread replacement = new Thread(() -> runCapturing(service, "replacement", backgroundFailure));
        replacement.start();
        assertThat(secondStarted.await(5, TimeUnit.SECONDS)).isTrue();

        releaseFirst.countDown();
        first.join(TimeUnit.SECONDS.toMillis(5));
        assertThat(service.isRunning()).isTrue();

        releaseSecond.countDown();
        replacement.join(TimeUnit.SECONDS.toMillis(5));
        assertThat(backgroundFailure.get()).isNull();
        assertThat(service.isRunning()).isFalse();
    }

    @Test
    void reportsNoResultsBeforeTheFirstRun() {
        KeepaliveService service = new KeepaliveService(this.registry, project -> {
            throw new SQLException("not called");
        }, this.properties);

        assertThat(service.lastResults()).isEmpty();
        assertThat(service.lastRun()).isEmpty();
        assertThat(service.projectCount()).isEqualTo(1);
    }

    @Test
    void pingsEveryConfiguredProject() throws SQLException {
        ProjectRegistry twoProjects = new ProjectRegistry(
                List.of(URL, "postgresql://postgres.def:pw@other.example.com:5432/postgres"), List.of("public.users"));
        Statement statement = statementReturningRows(1);
        KeepaliveService service = new KeepaliveService(twoProjects, project -> connectionFor(statement),
                this.properties);

        RunSummary summary = service.runAll("test");

        assertThat(summary.total()).isEqualTo(2);
        assertThat(summary.succeeded()).isEqualTo(2);
        assertThat(summary.results()).extracting(PingResult::projectId).containsExactly("p1", "p2");
    }

    private static KeepaliveProperties properties() {
        KeepaliveProperties properties = new KeepaliveProperties();
        properties.setRetryAttempts(3);
        properties.setRetryBackoffMs(0);
        properties.setQueryTimeoutSeconds(5);
        return properties;
    }

    private static Statement statementReturningRows(int rows) throws SQLException {
        ResultSet resultSet = mock(ResultSet.class);
        Boolean[] remaining = new Boolean[rows + 1];
        for (int i = 0; i < rows; i++) {
            remaining[i] = true;
        }
        remaining[rows] = false;
        when(resultSet.next()).thenReturn(remaining[0], java.util.Arrays.copyOfRange(remaining, 1, remaining.length));
        Statement statement = mock(Statement.class);
        when(statement.executeQuery(anyString())).thenReturn(resultSet);
        return statement;
    }

    private static Connection connectionFor(Statement statement) throws SQLException {
        Connection connection = mock(Connection.class);
        when(connection.createStatement()).thenReturn(statement);
        return connection;
    }

    private static void runCapturing(KeepaliveService service, String trigger, AtomicReference<Throwable> failure) {
        try {
            service.runAll(trigger);
        }
        catch (Throwable ex) {
            failure.compareAndSet(null, ex);
        }
    }

}
