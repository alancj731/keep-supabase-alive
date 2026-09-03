package com.jian.supabasekeepalive.config;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import org.springframework.boot.SpringApplication;
import org.springframework.core.env.MapPropertySource;
import org.springframework.mock.env.MockEnvironment;

import static org.assertj.core.api.Assertions.assertThat;

class DotenvEnvironmentPostProcessorTest {

    private final DotenvEnvironmentPostProcessor processor = new DotenvEnvironmentPostProcessor();

    @Test
    void parsesKeysCommentsAndExports() {
        Map<String, Object> values = DotenvEnvironmentPostProcessor.parse(List.of(
                "# a comment",
                "",
                "SUPABASE_TABLES=public.ping",
                "export KEEPALIVE_CRON=0 0 3 * * *",
                "   SPACED_KEY   =   spaced value   ",
                "NOT_A_PAIR",
                "=novalue"));

        assertThat(values).containsExactly(Map.entry("SUPABASE_TABLES", "public.ping"),
                Map.entry("KEEPALIVE_CRON", "0 0 3 * * *"), Map.entry("SPACED_KEY", "spaced value"));
    }

    @Test
    void handlesQuotesAndEscapes() {
        Map<String, Object> values = DotenvEnvironmentPostProcessor.parse(List.of(
                "DOUBLE=\"a b\\nc\"",
                "SINGLE='raw \\n value'",
                "HASH_IN_QUOTES=\"pass#word\"",
                "TRAILING_COMMENT=value # explanation"));

        assertThat(values).containsEntry("DOUBLE", "a b\nc")
                .containsEntry("SINGLE", "raw \\n value")
                .containsEntry("HASH_IN_QUOTES", "pass#word")
                .containsEntry("TRAILING_COMMENT", "value");
    }

    @Test
    void keepsSpecialCharactersInAConnectionString() {
        Map<String, Object> values = DotenvEnvironmentPostProcessor.parse(
                List.of("SUPABASE_URLS=postgresql://postgres.abc:p%40ss@aws-0-us-east-1.pooler.supabase.com:5432/postgres"));

        assertThat(values).containsEntry("SUPABASE_URLS",
                "postgresql://postgres.abc:p%40ss@aws-0-us-east-1.pooler.supabase.com:5432/postgres");
    }

    @Test
    void loadsTheFileAndRecordsItsPath(@TempDir Path dir) throws IOException {
        Path envFile = dir.resolve(".env");
        Files.writeString(envFile, "SUPABASE_TABLES=public.ping\n", StandardCharsets.UTF_8);
        MockEnvironment environment = new MockEnvironment();
        environment.setProperty("DOTENV_PATH", envFile.toString());

        this.processor.postProcessEnvironment(environment, new SpringApplication());

        assertThat(environment.getProperty("SUPABASE_TABLES")).isEqualTo("public.ping");
        assertThat(environment.getProperty(DotenvEnvironmentPostProcessor.SOURCE_PROPERTY))
                .isEqualTo(envFile.toAbsolutePath().normalize().toString());
    }

    @Test
    void toleratesAMissingFile(@TempDir Path dir) {
        MockEnvironment environment = new MockEnvironment();
        environment.setProperty("DOTENV_PATH", dir.resolve("absent.env").toString());

        this.processor.postProcessEnvironment(environment, new SpringApplication());

        assertThat(environment.getProperty(DotenvEnvironmentPostProcessor.SOURCE_PROPERTY)).isEqualTo("none");
    }

    @Test
    void doesNotOverrideAnExistingEnvironmentValue(@TempDir Path dir) throws IOException {
        Path envFile = dir.resolve(".env");
        Files.writeString(envFile, "SUPABASE_TABLES=from.file\n", StandardCharsets.UTF_8);
        MockEnvironment environment = new MockEnvironment();
        environment.setProperty("DOTENV_PATH", envFile.toString());
        environment.getPropertySources()
                .addFirst(new MapPropertySource("realEnv", Map.of("SUPABASE_TABLES", "from.environment")));

        this.processor.postProcessEnvironment(environment, new SpringApplication());

        assertThat(environment.getProperty("SUPABASE_TABLES")).isEqualTo("from.environment");
    }
}
