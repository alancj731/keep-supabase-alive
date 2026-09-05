package com.jian.supabasekeepalive.config;

import org.junit.jupiter.api.Test;

import org.springframework.boot.context.logging.LoggingApplicationListener;

import static org.assertj.core.api.Assertions.assertThat;

class DotenvApplicationListenerTest {

    /**
     * KEEPALIVE_LOG_LEVEL only works if .env is in the environment before Boot configures logging,
     * so this listener must sort ahead of LoggingApplicationListener.
     */
    @Test
    void runsBeforeLoggingIsConfigured() {
        assertThat(new DotenvApplicationListener().getOrder()).isLessThan(LoggingApplicationListener.DEFAULT_ORDER);
    }
}
