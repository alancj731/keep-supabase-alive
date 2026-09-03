package com.jian.supabasekeepalive.web;

import java.util.List;

import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.RunSummary;

/** Shape of POST /api/keepalive/run. */
public record RunResponse(StatusResponse.Run run, List<PingResult> projects) {

    static RunResponse from(RunSummary summary) {
        return new RunResponse(StatusResponse.Run.from(summary), summary.results());
    }
}
