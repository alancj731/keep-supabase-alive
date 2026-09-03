package com.jian.supabasekeepalive.config;

import org.springframework.boot.context.event.ApplicationEnvironmentPreparedEvent;
import org.springframework.context.ApplicationListener;
import org.springframework.core.Ordered;

/**
 * Applies {@link DotenvEnvironmentPostProcessor} as early as possible.
 *
 * <p>Ordering matters: Boot's {@code LoggingApplicationListener} reads {@code logging.level.*} from
 * the environment on this same event, so a {@code .env} loaded after it would leave
 * {@code KEEPALIVE_LOG_LEVEL} with no effect. Running at the highest precedence means the file is
 * in the environment before anything else consumes it.
 */
public class DotenvApplicationListener implements ApplicationListener<ApplicationEnvironmentPreparedEvent>, Ordered {

    private final DotenvEnvironmentPostProcessor processor = new DotenvEnvironmentPostProcessor();

    @Override
    public void onApplicationEvent(ApplicationEnvironmentPreparedEvent event) {
        this.processor.postProcessEnvironment(event.getEnvironment(), event.getSpringApplication());
    }

    @Override
    public int getOrder() {
        return Ordered.HIGHEST_PRECEDENCE;
    }
}
