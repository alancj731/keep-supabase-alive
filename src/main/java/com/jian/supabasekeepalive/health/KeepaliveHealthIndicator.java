package com.jian.supabasekeepalive.health;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.service.KeepaliveService;
import org.springframework.boot.health.contributor.Health;
import org.springframework.boot.health.contributor.HealthIndicator;
import org.springframework.stereotype.Component;

/**
 * Reports DOWN when any project's most recent keep-alive query failed, so a paused or
 * misconfigured project is visible on /actuator/health. Container health checks should use
 * /actuator/health/liveness instead — an unreachable Supabase project is not a reason to restart
 * this service.
 */
@Component("keepalive")
public class KeepaliveHealthIndicator implements HealthIndicator {

    private final KeepaliveService keepaliveService;

    public KeepaliveHealthIndicator(KeepaliveService keepaliveService) {
        this.keepaliveService = keepaliveService;
    }

    @Override
    public Health health() {
        List<PingResult> results = this.keepaliveService.lastResults();
        if (results.isEmpty()) {
            return Health.unknown()
                    .withDetail("message", "no keep-alive run has completed yet")
                    .withDetail("projects", this.keepaliveService.projectCount())
                    .build();
        }
        Map<String, Object> details = new LinkedHashMap<>();
        boolean anyFailed = false;
        for (PingResult result : results) {
            // Keyed by project id as well: two projects can share a host, user and table.
            details.put(result.projectId() + " " + result.projectName() + " [" + result.table() + "]", result.success()
                    ? "ok in " + result.durationMs() + " ms at " + result.checkedAt()
                    : "FAILED at " + result.checkedAt() + ": " + result.error());
            anyFailed |= !result.success();
        }
        Health.Builder builder = anyFailed ? Health.down() : Health.up();
        return builder.withDetails(details).build();
    }
}
