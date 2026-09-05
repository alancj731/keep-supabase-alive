package com.jian.supabasekeepalive.service;

import java.sql.Connection;
import java.sql.SQLException;

import com.jian.supabasekeepalive.model.SupabaseProject;

/** Opens a short-lived connection to one project. An interface so the service is testable without a database. */
public interface JdbcConnectionFactory {

    Connection open(SupabaseProject project) throws SQLException;
}
