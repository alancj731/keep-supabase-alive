package com.jian.supabasekeepalive.health;

import java.util.List;

import com.jian.supabasekeepalive.model.PingResult;
import com.jian.supabasekeepalive.model.SupabaseProject;
import com.jian.supabasekeepalive.service.KeepaliveService;
import org.junit.jupiter.api.Test;

import org.springframework.boot.health.contributor.Health;
import org.springframework.boot.health.contributor.Status;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.BDDMockito.given;
import static org.mockito.Mockito.mock;

class KeepaliveHealthIndicatorTest {

    private final KeepaliveService keepaliveService = mock(KeepaliveService.class);

    private final KeepaliveHealthIndicator indicator = new KeepaliveHealthIndicator(this.keepaliveService);

    @Test
    void isUnknownBeforeTheFirstRun() {
        given(this.keepaliveService.lastResults()).willReturn(List.of());
        given(this.keepaliveService.projectCount()).willReturn(2);

        Health health = this.indicator.health();

        assertThat(health.getStatus()).isEqualTo(Status.UNKNOWN);
        assertThat(health.getDetails()).containsEntry("projects", 2);
    }

    @Test
    void isUpWhenEveryProjectSucceeded() {
        given(this.keepaliveService.lastResults())
                .willReturn(List.of(success("p1", "public.a"), success("p2", "public.b")));

        Health health = this.indicator.health();

        assertThat(health.getStatus()).isEqualTo(Status.UP);
        assertThat(health.getDetails()).hasSize(2);
    }

    @Test
    void isDownWhenAnyProjectFailed() {
        given(this.keepaliveService.lastResults())
                .willReturn(List.of(success("p1", "public.a"), failure("p2", "public.b")));

        Health health = this.indicator.health();

        assertThat(health.getStatus()).isEqualTo(Status.DOWN);
        assertThat(health.getDetails().values()).anySatisfy(value -> assertThat(value).asString().contains("FAILED"));
    }

    @Test
    void keepsBothProjectsWhenTheyShareAHostAndTable() {
        given(this.keepaliveService.lastResults())
                .willReturn(List.of(success("p1", "public.same"), failure("p2", "public.same")));

        Health health = this.indicator.health();

        assertThat(health.getDetails()).hasSize(2);
        assertThat(health.getStatus()).isEqualTo(Status.DOWN);
    }

    private static PingResult success(String id, String table) {
        return PingResult.success(project(id, table), 1, 12, 1);
    }

    private static PingResult failure(String id, String table) {
        return PingResult.failure(project(id, table), 3, 30, "[SQLState 28P01] nope");
    }

    private static SupabaseProject project(String id, String table) {
        return new SupabaseProject(id, "postgres@db.example.com/postgres",
                "jdbc:postgresql://db.example.com:5432/postgres", "postgres", "pw", "db.example.com", 5432, "postgres",
                table, "\"public\".\"x\"");
    }
}
