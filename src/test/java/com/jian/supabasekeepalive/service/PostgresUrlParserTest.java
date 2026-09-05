package com.jian.supabasekeepalive.service;

import com.jian.supabasekeepalive.service.PostgresUrlParser.ConnectionTarget;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatIllegalArgumentException;

class PostgresUrlParserTest {

    @Test
    void parsesTheUriSupabaseShows() {
        ConnectionTarget target = PostgresUrlParser
                .parse("postgresql://postgres.abcdefgh:secretpw@aws-0-us-east-1.pooler.supabase.com:5432/postgres");

        assertThat(target.username()).isEqualTo("postgres.abcdefgh");
        assertThat(target.password()).isEqualTo("secretpw");
        assertThat(target.host()).isEqualTo("aws-0-us-east-1.pooler.supabase.com");
        assertThat(target.port()).isEqualTo(5432);
        assertThat(target.database()).isEqualTo("postgres");
        assertThat(target.jdbcUrl())
                .isEqualTo("jdbc:postgresql://aws-0-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require");
    }

    @Test
    void acceptsThePostgresScheme() {
        assertThat(PostgresUrlParser.parse("postgres://user:pw@db.example.com/postgres").host())
                .isEqualTo("db.example.com");
    }

    @Test
    void defaultsPortAndDatabase() {
        ConnectionTarget target = PostgresUrlParser.parse("postgresql://user:pw@db.example.com");

        assertThat(target.port()).isEqualTo(5432);
        assertThat(target.database()).isEqualTo("postgres");
        assertThat(target.jdbcUrl()).isEqualTo("jdbc:postgresql://db.example.com:5432/postgres?sslmode=require");
    }

    @Test
    void honoursExplicitPortAndDatabase() {
        ConnectionTarget target = PostgresUrlParser.parse("postgresql://user:pw@db.example.com:6543/appdb");

        assertThat(target.port()).isEqualTo(6543);
        assertThat(target.database()).isEqualTo("appdb");
    }

    @Test
    void decodesPercentEncodedCredentialsWithoutTouchingPlusSigns() {
        ConnectionTarget target = PostgresUrlParser.parse("postgresql://user:p%40ss%3Aw%2Cd+x@db.example.com/postgres");

        assertThat(target.password()).isEqualTo("p@ss:w,d+x");
    }

    @Test
    void keepsCredentialsOutOfTheJdbcUrl() {
        ConnectionTarget target = PostgresUrlParser.parse("postgresql://user:secretpw@db.example.com/postgres");

        assertThat(target.jdbcUrl()).doesNotContain("secretpw").doesNotContain("user");
        assertThat(target.toString()).contains("***").doesNotContain("secretpw");
    }

    @Test
    void preservesAnExplicitSslModeAndOtherParameters() {
        ConnectionTarget target = PostgresUrlParser
                .parse("postgresql://user:pw@db.example.com/postgres?sslmode=verify-full&ApplicationName=x");

        assertThat(target.jdbcUrl()).endsWith("?sslmode=verify-full&ApplicationName=x");
    }

    @Test
    void addsSslModeAlongsideExistingParameters() {
        ConnectionTarget target = PostgresUrlParser
                .parse("postgresql://user:pw@db.example.com/postgres?options=-c%20statement_timeout%3D5000");

        assertThat(target.jdbcUrl()).endsWith("?options=-c%20statement_timeout%3D5000&sslmode=require");
    }

    @Test
    void acceptsAJdbcUrlWithCredentialParameters() {
        ConnectionTarget target = PostgresUrlParser
                .parse("jdbc:postgresql://db.example.com:5432/postgres?user=admin&password=secretpw&ssl=true");

        assertThat(target.username()).isEqualTo("admin");
        assertThat(target.password()).isEqualTo("secretpw");
        assertThat(target.jdbcUrl()).isEqualTo("jdbc:postgresql://db.example.com:5432/postgres?ssl=true&sslmode=require");
    }

    @Test
    void rejectsAJdbcUrlWithoutCredentials() {
        assertThatIllegalArgumentException()
                .isThrownBy(() -> PostgresUrlParser.parse("jdbc:postgresql://db.example.com:5432/postgres"))
                .withMessageContaining("must carry user and password");
    }

    @Test
    void rejectsAUriWithoutCredentials() {
        assertThatIllegalArgumentException()
                .isThrownBy(() -> PostgresUrlParser.parse("postgresql://db.example.com:5432/postgres"))
                .withMessageContaining("no credentials");
    }

    @Test
    void rejectsAUriWithoutAPassword() {
        assertThatIllegalArgumentException()
                .isThrownBy(() -> PostgresUrlParser.parse("postgresql://user@db.example.com/postgres"))
                .withMessageContaining("no password");
    }

    @Test
    void rejectsAnUnknownScheme() {
        assertThatIllegalArgumentException()
                .isThrownBy(() -> PostgresUrlParser.parse("mysql://user:pw@db.example.com/db"))
                .withMessageContaining("unsupported connection string");
    }

    @Test
    void rejectsBlankInput() {
        assertThatIllegalArgumentException().isThrownBy(() -> PostgresUrlParser.parse("   "))
                .withMessageContaining("empty");
    }
}
