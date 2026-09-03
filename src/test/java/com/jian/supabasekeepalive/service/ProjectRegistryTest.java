package com.jian.supabasekeepalive.service;

import java.util.List;

import com.jian.supabasekeepalive.model.SupabaseProject;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatIllegalStateException;

class ProjectRegistryTest {

    private static final String URL_ONE = "postgresql://postgres.aaa:pw1@aws-0-us-east-1.pooler.supabase.com:5432/postgres";

    private static final String URL_TWO = "postgresql://postgres.bbb:pw2@aws-0-eu-west-2.pooler.supabase.com:5432/postgres";

    @Test
    void broadcastsASingleTableToEveryProject() {
        ProjectRegistry registry = new ProjectRegistry(List.of(URL_ONE, URL_TWO), List.of("public.ping"));

        assertThat(registry.size()).isEqualTo(2);
        assertThat(registry.projects()).extracting(SupabaseProject::table).containsExactly("public.ping", "public.ping");
        assertThat(registry.projects()).extracting(SupabaseProject::id).containsExactly("p1", "p2");
    }

    @Test
    void alignsOneTablePerProjectByPosition() {
        ProjectRegistry registry = new ProjectRegistry(List.of(URL_ONE, URL_TWO), List.of("public.a", "public.b"));

        assertThat(registry.projects()).extracting(SupabaseProject::table).containsExactly("public.a", "public.b");
        assertThat(registry.projects()).extracting(SupabaseProject::quotedTable)
                .containsExactly("\"public\".\"a\"", "\"public\".\"b\"");
    }

    @Test
    void ignoresBlankEntries() {
        ProjectRegistry registry = new ProjectRegistry(List.of(URL_ONE, "  ", URL_TWO), List.of("public.ping", ""));

        assertThat(registry.size()).isEqualTo(2);
    }

    @Test
    void rejectsAMismatchedNumberOfTables() {
        assertThatIllegalStateException()
                .isThrownBy(() -> new ProjectRegistry(List.of(URL_ONE, URL_TWO), List.of("a", "b", "c")))
                .withMessageContaining("3 entries")
                .withMessageContaining("2");
    }

    @Test
    void rejectsMissingUrls() {
        assertThatIllegalStateException().isThrownBy(() -> new ProjectRegistry(List.of(), List.of("public.ping")))
                .withMessageContaining("SUPABASE_URLS is empty");
    }

    @Test
    void rejectsMissingTables() {
        assertThatIllegalStateException().isThrownBy(() -> new ProjectRegistry(List.of(URL_ONE), List.of()))
                .withMessageContaining("SUPABASE_TABLES is empty");
    }

    @Test
    void reportsWhichEntryIsBrokenWithoutLeakingIt() {
        assertThatIllegalStateException()
                .isThrownBy(() -> new ProjectRegistry(List.of(URL_ONE, "postgresql://nopassword@host/db"),
                        List.of("public.ping")))
                .withMessageContaining("entry #2")
                .withMessageNotContaining("nopassword");
    }

    @Test
    void reportsABadTableName() {
        assertThatIllegalStateException()
                .isThrownBy(() -> new ProjectRegistry(List.of(URL_ONE), List.of("users; drop table x")))
                .withMessageContaining("SUPABASE_TABLES entry for project #1");
    }
}
