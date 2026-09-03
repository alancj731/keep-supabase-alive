package com.jian.supabasekeepalive.scheduler;

import com.jian.supabasekeepalive.config.KeepaliveProperties;
import com.jian.supabasekeepalive.service.KeepaliveService;
import com.jian.supabasekeepalive.service.RunInProgressException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

/** Triggers keep-alive runs on the configured cron, and optionally once at startup. */
@Component
public class KeepaliveScheduler {

    private static final Logger logger = LoggerFactory.getLogger(KeepaliveScheduler.class);

    private final KeepaliveService keepaliveService;

    private final KeepaliveProperties properties;

    public KeepaliveScheduler(KeepaliveService keepaliveService, KeepaliveProperties properties) {
        this.keepaliveService = keepaliveService;
        this.properties = properties;
    }

    @Scheduled(cron = "${keepalive.cron}", zone = "${keepalive.timezone}")
    public void runOnSchedule() {
        runQuietly("scheduled");
    }

    /** Verifies connectivity right after a deploy, off the startup thread so readiness is not delayed. */
    @EventListener(ApplicationReadyEvent.class)
    public void runOnStartup() {
        logger.info("Keep-alive configured for {} project(s); cron '{}' ({}); .env file: {}",
                this.keepaliveService.projectCount(), this.properties.getCron(), this.properties.getTimezone(),
                describeDotenvSource());
        if (!this.properties.isRunOnStartup()) {
            return;
        }
        Thread.ofVirtual().name("keepalive-startup").start(() -> runQuietly("startup"));
    }

    /** In a container the values normally arrive as environment variables, with no file involved. */
    private String describeDotenvSource() {
        String source = this.properties.getDotenvSource();
        return "none".equals(source) ? "none (using environment variables)" : source;
    }

    private void runQuietly(String trigger) {
        try {
            this.keepaliveService.runAll(trigger);
        }
        catch (RunInProgressException ex) {
            logger.warn("Skipping {} keep-alive run: {}", trigger, ex.getMessage());
        }
        catch (RuntimeException ex) {
            logger.error("Keep-alive run ({}) failed unexpectedly", trigger, ex);
        }
    }
}
