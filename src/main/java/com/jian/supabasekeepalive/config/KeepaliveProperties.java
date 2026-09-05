package com.jian.supabasekeepalive.config;

import java.util.ArrayList;
import java.util.List;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Bound from the environment (and therefore from {@code .env}) via {@code application.yml}.
 * Every field maps to an upper-case env var so the whole service is configured without code.
 */
@ConfigurationProperties(prefix = "keepalive")
public class KeepaliveProperties {

    /** Supabase connection strings, one per project. */
    private List<String> urls = new ArrayList<>();

    /** Either a single table applied to every project, or one table per URL in the same order. */
    private List<String> tables = new ArrayList<>();

    /** Spring cron expression; "-" disables scheduled runs. */
    private String cron = "0 0 3 * * *";

    private String timezone = "UTC";

    private boolean runOnStartup = true;

    private int retryAttempts = 3;

    private long retryBackoffMs = 2000;

    private int connectTimeoutSeconds = 10;

    private int queryTimeoutSeconds = 10;

    /** When non-blank, /api/** requires an {@code Authorization: Bearer <token>} header. */
    private String apiToken = "";

    /** Optional bearer token used by an external cron scheduler. */
    private String cronSecret = "";

    /** Age after which an in-flight run is assumed abandoned and may be taken over. */
    private long staleRunAfterSeconds = 300;

    /** Absolute path of the .env file that was loaded, or "none". Set by DotenvEnvironmentPostProcessor. */
    private String dotenvSource = "none";

    public List<String> getUrls() {
        return urls;
    }

    public void setUrls(List<String> urls) {
        this.urls = urls;
    }

    public List<String> getTables() {
        return tables;
    }

    public void setTables(List<String> tables) {
        this.tables = tables;
    }

    public String getCron() {
        return cron;
    }

    public void setCron(String cron) {
        this.cron = cron;
    }

    public String getTimezone() {
        return timezone;
    }

    public void setTimezone(String timezone) {
        this.timezone = timezone;
    }

    public boolean isRunOnStartup() {
        return runOnStartup;
    }

    public void setRunOnStartup(boolean runOnStartup) {
        this.runOnStartup = runOnStartup;
    }

    public int getRetryAttempts() {
        return retryAttempts;
    }

    public void setRetryAttempts(int retryAttempts) {
        this.retryAttempts = retryAttempts;
    }

    public long getRetryBackoffMs() {
        return retryBackoffMs;
    }

    public void setRetryBackoffMs(long retryBackoffMs) {
        this.retryBackoffMs = retryBackoffMs;
    }

    public int getConnectTimeoutSeconds() {
        return connectTimeoutSeconds;
    }

    public void setConnectTimeoutSeconds(int connectTimeoutSeconds) {
        this.connectTimeoutSeconds = connectTimeoutSeconds;
    }

    public int getQueryTimeoutSeconds() {
        return queryTimeoutSeconds;
    }

    public void setQueryTimeoutSeconds(int queryTimeoutSeconds) {
        this.queryTimeoutSeconds = queryTimeoutSeconds;
    }

    public String getApiToken() {
        return apiToken;
    }

    public void setApiToken(String apiToken) {
        this.apiToken = apiToken;
    }

    public String getCronSecret() {
        return cronSecret;
    }

    public void setCronSecret(String cronSecret) {
        this.cronSecret = cronSecret;
    }

    public long getStaleRunAfterSeconds() {
        return staleRunAfterSeconds;
    }

    public void setStaleRunAfterSeconds(long staleRunAfterSeconds) {
        this.staleRunAfterSeconds = staleRunAfterSeconds;
    }

    public String getDotenvSource() {
        return dotenvSource;
    }

    public void setDotenvSource(String dotenvSource) {
        this.dotenvSource = dotenvSource;
    }
}
