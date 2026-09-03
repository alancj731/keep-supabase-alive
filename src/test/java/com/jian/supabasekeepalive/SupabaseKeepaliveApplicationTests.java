package com.jian.supabasekeepalive;

import com.jian.supabasekeepalive.service.KeepaliveService;
import com.jian.supabasekeepalive.service.ProjectRegistry;
import org.junit.jupiter.api.Test;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Boots the whole application with two fake projects to prove the wiring: no DataSource
 * autoconfiguration, the .env-backed properties bind, and the registry parses the URLs. Nothing
 * here touches the network — the schedule is disabled and the startup ping is off.
 */
@SpringBootTest(properties = {
        "keepalive.urls=postgresql://postgres.aaa:pw1@aws-0-us-east-1.pooler.supabase.com:5432/postgres,"
                + "postgresql://postgres.bbb:pw2@aws-0-eu-west-2.pooler.supabase.com:6543/postgres",
        "keepalive.tables=public.keepalive_ping",
        "keepalive.cron=-",
        "keepalive.run-on-startup=false" })
class SupabaseKeepaliveApplicationTests {

    @Autowired
    private ProjectRegistry registry;

    @Autowired
    private KeepaliveService keepaliveService;

    @Test
    void contextLoadsWithTheConfiguredProjects() {
        assertThat(this.registry.size()).isEqualTo(2);
        assertThat(this.registry.projects().getFirst().jdbcUrl())
                .isEqualTo("jdbc:postgresql://aws-0-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require");
        assertThat(this.registry.projects().getLast().port()).isEqualTo(6543);
        assertThat(this.keepaliveService.projectCount()).isEqualTo(2);
        assertThat(this.keepaliveService.lastRun()).isEmpty();
    }
}
