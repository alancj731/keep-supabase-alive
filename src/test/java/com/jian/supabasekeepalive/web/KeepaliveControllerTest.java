package com.jian.supabasekeepalive.web;

import java.time.Instant;
import java.util.List;

import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.RunSummary;
import com.jian.supabasekeepalive.model.SupabaseProject;
import com.jian.supabasekeepalive.service.KeepaliveService;
import com.jian.supabasekeepalive.service.RunInProgressException;
import org.junit.jupiter.api.Test;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.BDDMockito.given;
import static org.mockito.BDDMockito.willThrow;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(KeepaliveController.class)
@org.springframework.test.context.TestPropertySource(
        properties = { "keepalive.cron=0 0 3 * * *", "keepalive.timezone=UTC" })
class KeepaliveControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockitoBean
    private KeepaliveService keepaliveService;

    @Test
    void statusReportsTheScheduleAndTheLastResults() throws Exception {
        given(this.keepaliveService.projectCount()).willReturn(2);
        given(this.keepaliveService.isRunning()).willReturn(false);
        given(this.keepaliveService.lastResults()).willReturn(List.of(successResult(), failureResult()));
        given(this.keepaliveService.lastRun()).willReturn(java.util.Optional.of(summary()));

        this.mockMvc.perform(get("/api/keepalive/status"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.projectCount").value(2))
                .andExpect(jsonPath("$.running").value(false))
                .andExpect(jsonPath("$.schedule.cron").value("0 0 3 * * *"))
                .andExpect(jsonPath("$.schedule.timezone").value("UTC"))
                .andExpect(jsonPath("$.schedule.nextRunAt").exists())
                .andExpect(jsonPath("$.lastRun.succeeded").value(1))
                .andExpect(jsonPath("$.lastRun.failed").value(1))
                .andExpect(jsonPath("$.projects.length()").value(2))
                .andExpect(jsonPath("$.projects[0].projectId").value("p1"))
                .andExpect(jsonPath("$.projects[0].success").value(true))
                .andExpect(jsonPath("$.projects[1].error").value("[SQLState 28P01] nope"));
    }

    @Test
    void statusWorksBeforeTheFirstRun() throws Exception {
        given(this.keepaliveService.projectCount()).willReturn(1);
        given(this.keepaliveService.lastResults()).willReturn(List.of());
        given(this.keepaliveService.lastRun()).willReturn(java.util.Optional.empty());

        this.mockMvc.perform(get("/api/keepalive/status"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.lastRun").doesNotExist())
                .andExpect(jsonPath("$.projects.length()").value(0));
    }

    @Test
    void runTriggersAKeepalivePass() throws Exception {
        given(this.keepaliveService.runAll("manual")).willReturn(summary());

        this.mockMvc.perform(post("/api/keepalive/run"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.run.trigger").value("manual"))
                .andExpect(jsonPath("$.run.total").value(2))
                .andExpect(jsonPath("$.projects.length()").value(2));
    }

    @Test
    void runAcceptsGetForExternalSchedulers() throws Exception {
        given(this.keepaliveService.runAll("manual")).willReturn(summary());

        this.mockMvc.perform(get("/api/keepalive/run"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.run.trigger").value("manual"));
    }

    @Test
    void runReturnsConflictWhileAnotherRunIsInFlight() throws Exception {
        willThrow(new RunInProgressException()).given(this.keepaliveService).runAll("manual");

        this.mockMvc.perform(post("/api/keepalive/run"))
                .andExpect(status().isConflict())
                .andExpect(jsonPath("$.error").value("a keep-alive run is already in progress"));
    }

    private static RunSummary summary() {
        Instant started = Instant.parse("2026-09-03T03:00:00Z");
        return RunSummary.of("manual", started, started.plusMillis(420),
                List.of(successResult(), failureResult()));
    }

    private static PingResult successResult() {
        return PingResult.success(project("p1", "public.users"), 1, 120, 1);
    }

    private static PingResult failureResult() {
        return PingResult.failure(project("p2", "public.events"), 3, 300, "[SQLState 28P01] nope");
    }

    private static SupabaseProject project(String id, String table) {
        return new SupabaseProject(id, "postgres." + id + "@db.example.com/postgres",
                "jdbc:postgresql://db.example.com:5432/postgres?sslmode=require", "postgres." + id, "pw",
                "db.example.com", 5432, "postgres", table, "\"public\".\"x\"");
    }
}
