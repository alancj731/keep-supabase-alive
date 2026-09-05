package com.jian.supabasekeepalive.web;

import java.time.Instant;
import java.util.List;

import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.RunSummary;

/** Shape of GET /api/keepalive/status. */
public record StatusResponse(
        Instant generatedAt,
        boolean running,
        int projectCount,
        Schedule schedule,
        Run lastRun,
        List<PingResult> projects) {

    public record Schedule(String cron, String timezone, Instant nextRunAt) {
    }

    /** The run metadata without its results, which are returned once under "projects". */
    public record Run(String trigger, Instant startedAt, Instant finishedAt, long durationMs, int total, int succeeded,
            int failed) {

        static Run from(RunSummary summary) {
            return new Run(summary.trigger(), summary.startedAt(), summary.finishedAt(), summary.durationMs(),
                    summary.total(), summary.succeeded(), summary.failed());
        }
    }
}
