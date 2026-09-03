package com.jian.supabasekeepalive.service;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.Properties;

import com.jian.supabasekeepalive.config.KeepaliveProperties;
import com.jian.supabasekeepalive.model.SupabaseProject;

/**
 * Opens a plain {@link DriverManager} connection per ping. A pool would keep idle connections open
 * against every project all day for the sake of one query, which is the opposite of what this
 * service is for.
 */
public class DriverManagerConnectionFactory implements JdbcConnectionFactory {

    private final KeepaliveProperties properties;

    public DriverManagerConnectionFactory(KeepaliveProperties properties) {
        this.properties = properties;
    }

    @Override
    public Connection open(SupabaseProject project) throws SQLException {
        Properties props = new Properties();
        props.setProperty("user", project.username());
        props.setProperty("password", project.password());
        props.setProperty("connectTimeout", String.valueOf(this.properties.getConnectTimeoutSeconds()));
        props.setProperty("loginTimeout", String.valueOf(this.properties.getConnectTimeoutSeconds()));
        props.setProperty("socketTimeout",
                String.valueOf(this.properties.getConnectTimeoutSeconds() + this.properties.getQueryTimeoutSeconds()));
        props.setProperty("ApplicationName", "supabase-keepalive");
        return DriverManager.getConnection(project.jdbcUrl(), props);
    }
}
