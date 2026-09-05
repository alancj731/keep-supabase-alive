package com.jian.supabasekeepalive.service;

import java.util.regex.Pattern;

/**
 * Validates and quotes a table name. A table cannot be a JDBC bind parameter, so the name is
 * whitelisted against a strict pattern and then double-quoted before it is ever interpolated into
 * SQL. Anything outside the pattern fails at startup rather than reaching the database.
 */
public final class TableIdentifier {

    private static final Pattern PART = Pattern.compile("[A-Za-z_][A-Za-z0-9_$]{0,62}");

    private TableIdentifier() {
    }

    /** Turns {@code public.users} into {@code "public"."users"}. */
    public static String quote(String table) {
        String trimmed = (table == null) ? "" : table.trim();
        if (trimmed.isEmpty()) {
            throw new IllegalArgumentException("table name must not be blank");
        }
        String[] parts = trimmed.split("\\.", -1);
        if (parts.length > 2) {
            throw new IllegalArgumentException(
                    "invalid table name '" + trimmed + "': expected 'table' or 'schema.table'");
        }
        StringBuilder quoted = new StringBuilder(trimmed.length() + 6);
        for (int i = 0; i < parts.length; i++) {
            if (!PART.matcher(parts[i]).matches()) {
                throw new IllegalArgumentException("invalid table name '" + trimmed
                        + "': each part must start with a letter or underscore and contain only "
                        + "letters, digits, underscore or $");
            }
            if (i > 0) {
                quoted.append('.');
            }
            quoted.append('"').append(parts[i]).append('"');
        }
        return quoted.toString();
    }
}
