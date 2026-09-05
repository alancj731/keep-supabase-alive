package com.jian.supabasekeepalive.web;

import java.time.Instant;
import java.time.ZoneId;
import java.time.ZonedDateTime;
import java.util.Map;

import com.jian.supabasekeepalive.config.KeepaliveProperties;
import com.jian.supabasekeepalive.service.KeepaliveService;
import com.jian.supabasekeepalive.service.RunInProgressException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.scheduling.support.CronExpression;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestMethod;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/keepalive")
public class KeepaliveController {

    private static final Logger logger = LoggerFactory.getLogger(KeepaliveController.class);

    private final KeepaliveService keepaliveService;

    private final KeepaliveProperties properties;

    public KeepaliveController(KeepaliveService keepaliveService, KeepaliveProperties properties) {
        this.keepaliveService = keepaliveService;
        this.properties = properties;
    }

    @GetMapping("/status")
    public StatusResponse status() {
        return new StatusResponse(Instant.now(), this.keepaliveService.isRunning(),
                this.keepaliveService.projectCount(), schedule(),
                this.keepaliveService.lastRun().map(StatusResponse.Run::from).orElse(null),
                this.keepaliveService.lastResults());
    }

    /** POST is the normal manual trigger; GET also supports hosted cron schedulers. */
    @RequestMapping(value = "/run", method = { RequestMethod.GET, RequestMethod.POST })
    public RunResponse run() {
        return RunResponse.from(this.keepaliveService.runAll("manual"));
    }

    @ExceptionHandler(RunInProgressException.class)
    ResponseEntity<Map<String, String>> handleRunInProgress(RunInProgressException ex) {
        return ResponseEntity.status(HttpStatus.CONFLICT).body(Map.of("error", ex.getMessage()));
    }

    private StatusResponse.Schedule schedule() {
        String cron = this.properties.getCron();
        String timezone = this.properties.getTimezone();
        return new StatusResponse.Schedule(cron, timezone, nextRunAt(cron, timezone));
    }

    private static Instant nextRunAt(String cron, String timezone) {
        if (cron == null || cron.isBlank() || "-".equals(cron.trim())) {
            return null;
        }
        try {
            ZonedDateTime next = CronExpression.parse(cron.trim()).next(ZonedDateTime.now(ZoneId.of(timezone)));
            return (next == null) ? null : next.toInstant();
        }
        catch (RuntimeException ex) {
            logger.warn("Cannot compute the next run time from cron '{}' in zone '{}': {}", cron, timezone,
                    ex.getMessage());
            return null;
        }
    }
}
