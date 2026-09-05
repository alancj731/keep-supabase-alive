package com.jian.supabasekeepalive;

import com.jian.supabasekeepalive.config.DotenvApplicationListener;
import com.jian.supabasekeepalive.config.KeepaliveProperties;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.scheduling.annotation.EnableScheduling;

@SpringBootApplication
@EnableScheduling
@EnableConfigurationProperties(KeepaliveProperties.class)
public class SupabaseKeepaliveApplication {

    public static void main(String[] args) {
        SpringApplication application = new SpringApplication(SupabaseKeepaliveApplication.class);
        // Registered explicitly rather than through a META-INF/spring/*.imports file: the
        // repackaged jar keeps META-INF at the archive root, where it is not on the application
        // classpath, so factories discovery would silently not happen for the jar deployment.
        application.addListeners(new DotenvApplicationListener());
        application.run(args);
    }
}
