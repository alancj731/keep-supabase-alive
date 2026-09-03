package com.jian.supabasekeepalive.model;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

/** Aggregate outcome of one pass over every configured project. */
public record RunSummary(
        String trigger,
        Instant startedAt,
        Instant finishedAt,
        long durationMs,
        int total,
        int succeeded,
        int failed,
        List<PingResult> results) {

    public static RunSummary of(String trigger, Instant startedAt, Instant finishedAt, List<PingResult> results) {
        int succeeded = (int) results.stream().filter(PingResult::success).count();
        return new RunSummary(trigger, startedAt, finishedAt, Duration.between(startedAt, finishedAt).toMillis(),
                results.size(), succeeded, results.size() - succeeded, List.copyOf(results));
    }
}
