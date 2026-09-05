package com.jian.supabasekeepalive.model;

/**
 * One Supabase project to keep alive, derived from a single {@code SUPABASE_URLS} entry.
 *
 * <p>{@code toString} deliberately omits the password so a project can be logged safely; the value
 * is also never included in the {@code jdbcUrl} (it is passed as a connection property instead).
 */
public record SupabaseProject(
        String id,
        String name,
        String jdbcUrl,
        String username,
        String password,
        String host,
        int port,
        String database,
        String table,
        String quotedTable) {

    @Override
    public String toString() {
        return "SupabaseProject[id=%s, name=%s, jdbcUrl=%s, username=%s, password=***, table=%s]"
                .formatted(id, name, jdbcUrl, username, table);
    }
}
