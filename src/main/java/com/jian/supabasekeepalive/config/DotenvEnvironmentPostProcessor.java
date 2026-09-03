package com.jian.supabasekeepalive.config;

import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import org.springframework.boot.EnvironmentPostProcessor;
import org.springframework.boot.SpringApplication;
import org.springframework.core.env.ConfigurableEnvironment;
import org.springframework.core.env.MapPropertySource;

/**
 * Loads a {@code .env} file into the Spring {@code Environment}.
 *
 * <p>The property source is added <em>last</em>, so real environment variables (for example those
 * injected by Docker Compose) always win over the file. The file location comes from
 * {@code DOTENV_PATH} when set, otherwise {@code ./.env}; a missing file is not an error.
 *
 * <p>Registered by hand in {@code SupabaseKeepaliveApplication#main}, not through a
 * {@code META-INF/spring/*.imports} file — see the note there.
 */
public class DotenvEnvironmentPostProcessor implements EnvironmentPostProcessor {

    public static final String PROPERTY_SOURCE_NAME = "dotenvFile";

    /** Records where the values came from so the application can log it once at startup. */
    public static final String SOURCE_PROPERTY = "keepalive.dotenv-source";

    /** Overrides the file location; read as a system property first, then an environment variable. */
    public static final String PATH_VARIABLE = "DOTENV_PATH";

    private static final String DEFAULT_FILENAME = ".env";

    @Override
    public void postProcessEnvironment(ConfigurableEnvironment environment, SpringApplication application) {
        Path path = resolvePath(environment);
        Map<String, Object> values = new LinkedHashMap<>();
        String source = "none";
        if (Files.isRegularFile(path)) {
            try {
                values.putAll(parse(Files.readAllLines(path, StandardCharsets.UTF_8)));
                source = path.toAbsolutePath().normalize().toString();
            }
            catch (IOException ex) {
                throw new UncheckedIOException("Unable to read " + path.toAbsolutePath(), ex);
            }
        }
        values.put(SOURCE_PROPERTY, source);
        environment.getPropertySources().addLast(new MapPropertySource(PROPERTY_SOURCE_NAME, values));
    }

    /**
     * The file location has to be known before the environment is fully populated, so this reads
     * {@code DOTENV_PATH} straight from the JVM system properties and the OS environment rather
     * than through {@code Environment}, which is still being assembled at this point.
     */
    private Path resolvePath(ConfigurableEnvironment environment) {
        for (String candidate : new String[] { System.getProperty(PATH_VARIABLE), System.getenv(PATH_VARIABLE),
                environment.getProperty(PATH_VARIABLE) }) {
            if (candidate != null && !candidate.isBlank()) {
                return Path.of(candidate.trim());
            }
        }
        return Path.of(DEFAULT_FILENAME);
    }

    /**
     * Parses dotenv syntax: {@code KEY=VALUE}, optional {@code export } prefix, {@code #} comments,
     * and single- or double-quoted values (escapes are only expanded inside double quotes).
     */
    static Map<String, Object> parse(List<String> lines) {
        Map<String, Object> values = new LinkedHashMap<>();
        for (String line : lines) {
            String trimmed = line.strip();
            if (trimmed.isEmpty() || trimmed.startsWith("#")) {
                continue;
            }
            if (trimmed.startsWith("export ")) {
                trimmed = trimmed.substring("export ".length()).strip();
            }
            int separator = trimmed.indexOf('=');
            if (separator <= 0) {
                continue;
            }
            String key = trimmed.substring(0, separator).strip();
            if (key.isEmpty()) {
                continue;
            }
            values.put(key, unquote(trimmed.substring(separator + 1).strip()));
        }
        return values;
    }

    private static String unquote(String value) {
        if (value.length() >= 2 && value.startsWith("\"") && value.endsWith("\"")) {
            return unescape(value.substring(1, value.length() - 1));
        }
        if (value.length() >= 2 && value.startsWith("'") && value.endsWith("'")) {
            return value.substring(1, value.length() - 1);
        }
        // Unquoted values end at an inline " #" comment; quote the value to keep a literal '#'.
        int comment = value.indexOf(" #");
        return (comment >= 0) ? value.substring(0, comment).stripTrailing() : value;
    }

    private static String unescape(String value) {
        StringBuilder out = new StringBuilder(value.length());
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            if (c == '\\' && i + 1 < value.length()) {
                char next = value.charAt(++i);
                switch (next) {
                    case 'n' -> out.append('\n');
                    case 'r' -> out.append('\r');
                    case 't' -> out.append('\t');
                    case '"' -> out.append('"');
                    case '\\' -> out.append('\\');
                    default -> out.append('\\').append(next);
                }
            }
            else {
                out.append(c);
            }
        }
        return out.toString();
    }
}
