package com.jian.supabasekeepalive.config;

import com.jian.supabasekeepalive.service.DriverManagerConnectionFactory;
import com.jian.supabasekeepalive.service.JdbcConnectionFactory;
import com.jian.supabasekeepalive.service.KeepaliveService;
import com.jian.supabasekeepalive.service.ProjectRegistry;
import com.jian.supabasekeepalive.web.ApiTokenFilter;
import org.springframework.boot.web.servlet.FilterRegistrationBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration(proxyBeanMethods = false)
public class KeepaliveConfiguration {

    /** Parses and validates the configuration; a bad .env fails the context start, by design. */
    @Bean
    ProjectRegistry projectRegistry(KeepaliveProperties properties) {
        try {
            return new ProjectRegistry(properties.getUrls(), properties.getTables());
        }
        catch (IllegalStateException ex) {
            // Name the file the values came from: the usual cause is an unread or misplaced .env.
            throw new IllegalStateException(
                    ex.getMessage() + " (.env source: " + properties.getDotenvSource() + ")", ex);
        }
    }

    @Bean
    JdbcConnectionFactory jdbcConnectionFactory(KeepaliveProperties properties) {
        return new DriverManagerConnectionFactory(properties);
    }

    @Bean
    KeepaliveService keepaliveService(ProjectRegistry registry, JdbcConnectionFactory connectionFactory,
            KeepaliveProperties properties) {
        return new KeepaliveService(registry, connectionFactory, properties);
    }

    @Bean
    FilterRegistrationBean<ApiTokenFilter> apiTokenFilter(KeepaliveProperties properties) {
        String token = (properties.getApiToken() == null) ? "" : properties.getApiToken().trim();
        String cronSecret = (properties.getCronSecret() == null) ? "" : properties.getCronSecret().trim();
        FilterRegistrationBean<ApiTokenFilter> registration =
                new FilterRegistrationBean<>(new ApiTokenFilter(token, cronSecret));
        registration.addUrlPatterns("/api/*");
        registration.setEnabled(!token.isEmpty() || !cronSecret.isEmpty());
        return registration;
    }
}
