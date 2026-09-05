package com.jian.supabasekeepalive.service;

import java.util.ArrayList;
import java.util.List;

import com.jian.supabasekeepalive.model.SupabaseProject;

/**
 * Pairs the configured connection strings with the configured tables.
 *
 * <p>A single table is broadcast to every project; otherwise the number of tables must match the
 * number of URLs and the two lists are aligned by position. Any problem throws here, at startup,
 * so a misconfigured service never boots into a state where it silently pings nothing.
 */
public class ProjectRegistry {

    private final List<SupabaseProject> projects;

    public ProjectRegistry(List<String> urls, List<String> tables) {
        List<String> cleanUrls = clean(urls);
        List<String> cleanTables = clean(tables);
        if (cleanUrls.isEmpty()) {
            throw new IllegalStateException("SUPABASE_URLS is empty: set at least one Supabase connection "
                    + "string (comma-separated) in .env");
        }
        if (cleanTables.isEmpty()) {
            throw new IllegalStateException("SUPABASE_TABLES is empty: set one table name for all projects, "
                    + "or one per project (comma-separated) in .env");
        }
        if (cleanTables.size() != 1 && cleanTables.size() != cleanUrls.size()) {
            throw new IllegalStateException("SUPABASE_TABLES has %d entries but SUPABASE_URLS has %d: supply "
                    .formatted(cleanTables.size(), cleanUrls.size())
                    + "either one table for all projects or exactly one per project, in the same order");
        }

        List<SupabaseProject> parsed = new ArrayList<>(cleanUrls.size());
        for (int i = 0; i < cleanUrls.size(); i++) {
            String table = (cleanTables.size() == 1) ? cleanTables.get(0) : cleanTables.get(i);
            // Never echo the raw entry in an error: it contains the database password.
            PostgresUrlParser.ConnectionTarget target;
            try {
                target = PostgresUrlParser.parse(cleanUrls.get(i));
            }
            catch (IllegalArgumentException ex) {
                throw new IllegalStateException(
                        "SUPABASE_URLS entry #" + (i + 1) + " is invalid: " + ex.getMessage(), ex);
            }
            String quotedTable;
            try {
                quotedTable = TableIdentifier.quote(table);
            }
            catch (IllegalArgumentException ex) {
                throw new IllegalStateException(
                        "SUPABASE_TABLES entry for project #" + (i + 1) + " is invalid: " + ex.getMessage(), ex);
            }
            parsed.add(new SupabaseProject("p" + (i + 1), target.name(), target.jdbcUrl(), target.username(),
                    target.password(), target.host(), target.port(), target.database(), table.trim(), quotedTable));
        }
        this.projects = List.copyOf(parsed);
    }

    public List<SupabaseProject> projects() {
        return this.projects;
    }

    public int size() {
        return this.projects.size();
    }

    private static List<String> clean(List<String> values) {
        if (values == null) {
            return List.of();
        }
        return values.stream().filter(v -> v != null && !v.isBlank()).map(String::trim).toList();
    }
}
