package com.jian.supabasekeepalive.service;

import java.io.ByteArrayOutputStream;
import java.net.URI;
import java.net.URISyntaxException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/**
 * Turns a Supabase connection string into the pieces the JDBC driver needs.
 *
 * <p>Accepts the {@code postgresql://user:password@host:port/db} URI that Supabase shows in
 * Project Settings, the {@code postgres://} alias, and a ready-made {@code jdbc:postgresql://} URL
 * carrying {@code user}/{@code password} query parameters. Credentials are always returned
 * separately from the JDBC URL so the URL itself is safe to log, and {@code sslmode=require} is
 * added when the caller did not specify one (Supabase requires TLS).
 */
public final class PostgresUrlParser {

    private static final int DEFAULT_PORT = 5432;

    private static final String DEFAULT_DATABASE = "postgres";

    private static final String JDBC_PREFIX = "jdbc:postgresql://";

    private PostgresUrlParser() {
    }

    /** Connection details for one project, without the table it should query. */
    public record ConnectionTarget(
            String name,
            String jdbcUrl,
            String username,
            String password,
            String host,
            int port,
            String database) {

        @Override
        public String toString() {
            return "ConnectionTarget[name=%s, jdbcUrl=%s, username=%s, password=***]"
                    .formatted(name, jdbcUrl, username);
        }
    }

    public static ConnectionTarget parse(String connectionString) {
        String raw = (connectionString == null) ? "" : connectionString.trim();
        if (raw.isEmpty()) {
            throw new IllegalArgumentException("connection string is empty");
        }
        if (raw.startsWith(JDBC_PREFIX)) {
            return parseJdbcUrl(raw);
        }
        if (raw.startsWith("postgresql://") || raw.startsWith("postgres://")) {
            return parseUri(raw);
        }
        throw new IllegalArgumentException("unsupported connection string: expected it to start with "
                + "postgresql://, postgres:// or " + JDBC_PREFIX);
    }

    private static ConnectionTarget parseUri(String raw) {
        URI uri = toUri(raw);
        String userInfo = uri.getRawUserInfo();
        if (userInfo == null || userInfo.isBlank()) {
            throw new IllegalArgumentException("connection string has no credentials: expected "
                    + "postgresql://user:password@host:port/database");
        }
        int colon = userInfo.indexOf(':');
        if (colon < 0) {
            throw new IllegalArgumentException("connection string has a user but no password");
        }
        String username = percentDecode(userInfo.substring(0, colon));
        String password = percentDecode(userInfo.substring(colon + 1));
        if (username.isEmpty() || password.isEmpty()) {
            throw new IllegalArgumentException("connection string has an empty user or password");
        }
        return build(uri, username, password, uri.getRawQuery());
    }

    private static ConnectionTarget parseJdbcUrl(String raw) {
        URI uri = toUri(raw.substring("jdbc:".length()));
        List<String> kept = new ArrayList<>();
        String username = null;
        String password = null;
        for (String param : splitQuery(uri.getRawQuery())) {
            int eq = param.indexOf('=');
            String key = (eq < 0) ? param : param.substring(0, eq);
            String value = (eq < 0) ? "" : param.substring(eq + 1);
            if ("user".equalsIgnoreCase(key)) {
                username = percentDecode(value);
            }
            else if ("password".equalsIgnoreCase(key)) {
                password = percentDecode(value);
            }
            else {
                kept.add(param);
            }
        }
        if (username == null || username.isEmpty() || password == null || password.isEmpty()) {
            throw new IllegalArgumentException(
                    "jdbc:postgresql:// URL must carry user and password query parameters");
        }
        return build(uri, username, password, String.join("&", kept));
    }

    private static ConnectionTarget build(URI uri, String username, String password, String query) {
        String host = uri.getHost();
        if (host == null || host.isBlank()) {
            throw new IllegalArgumentException("connection string has no host; percent-encode any "
                    + "@ : / ? # or , characters in the password (for example @ becomes %40)");
        }
        int port = (uri.getPort() > 0) ? uri.getPort() : DEFAULT_PORT;
        String path = (uri.getPath() == null) ? "" : uri.getPath();
        String database = path.startsWith("/") ? path.substring(1) : path;
        if (database.isBlank()) {
            database = DEFAULT_DATABASE;
        }
        String effectiveQuery = ensureSslMode(query);
        String jdbcUrl = JDBC_PREFIX + host + ":" + port + "/" + database
                + (effectiveQuery.isEmpty() ? "" : "?" + effectiveQuery);
        return new ConnectionTarget(username + "@" + host + "/" + database, jdbcUrl, username, password, host, port,
                database);
    }

    private static String ensureSslMode(String query) {
        List<String> params = splitQuery(query);
        for (String param : params) {
            int eq = param.indexOf('=');
            String key = (eq < 0) ? param : param.substring(0, eq);
            if ("sslmode".equalsIgnoreCase(key)) {
                return String.join("&", params);
            }
        }
        params.add("sslmode=require");
        return String.join("&", params);
    }

    private static List<String> splitQuery(String query) {
        List<String> params = new ArrayList<>();
        if (query == null || query.isBlank()) {
            return params;
        }
        for (String param : query.split("&")) {
            if (!param.isBlank()) {
                params.add(param);
            }
        }
        return params;
    }

    private static URI toUri(String raw) {
        try {
            return new URI(raw);
        }
        catch (URISyntaxException ex) {
            throw new IllegalArgumentException("connection string is not a valid URI: " + ex.getReason()
                    + " (percent-encode any @ : / ? # or , characters in the password)");
        }
    }

    /**
     * Decodes %XX escapes only. {@code URLDecoder} would additionally turn '+' into a space, which
     * would corrupt any password containing a plus sign.
     */
    private static String percentDecode(String value) {
        if (value.indexOf('%') < 0) {
            return value;
        }
        ByteArrayOutputStream out = new ByteArrayOutputStream(value.length());
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            if (c == '%') {
                if (i + 2 >= value.length()) {
                    throw new IllegalArgumentException("connection string has a truncated %-escape");
                }
                int high = Character.digit(value.charAt(i + 1), 16);
                int low = Character.digit(value.charAt(i + 2), 16);
                if (high < 0 || low < 0) {
                    throw new IllegalArgumentException("connection string has an invalid %-escape");
                }
                out.write((high << 4) + low);
                i += 2;
            }
            else {
                byte[] bytes = String.valueOf(c).getBytes(StandardCharsets.UTF_8);
                out.write(bytes, 0, bytes.length);
            }
        }
        return out.toString(StandardCharsets.UTF_8);
    }
}
