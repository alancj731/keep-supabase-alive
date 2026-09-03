package com.jian.supabasekeepalive.model;

import java.time.Instant;

/** Outcome of a single keep-alive query against one project. */
public record PingResult(
        String projectId,
        String projectName,
        String host,
        int port,
        String database,
        String table,
        boolean success,
        int attempts,
        long durationMs,
        Integer rowsSeen,
        String error,
        Instant checkedAt) {

    public static PingResult success(SupabaseProject project, int attempts, long durationMs, int rowsSeen) {
        return new PingResult(project.id(), project.name(), project.host(), project.port(), project.database(),
                project.table(), true, attempts, durationMs, rowsSeen, null, Instant.now());
    }

    public static PingResult failure(SupabaseProject project, int attempts, long durationMs, String error) {
        return new PingResult(project.id(), project.name(), project.host(), project.port(), project.database(),
                project.table(), false, attempts, durationMs, null, error, Instant.now());
    }
}
