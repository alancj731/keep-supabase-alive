package com.jian.supabasekeepalive.service;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatIllegalArgumentException;

class TableIdentifierTest {

    @Test
    void quotesASchemaQualifiedName() {
        assertThat(TableIdentifier.quote("public.users")).isEqualTo("\"public\".\"users\"");
    }

    @Test
    void quotesABareName() {
        assertThat(TableIdentifier.quote("users")).isEqualTo("\"users\"");
    }

    @Test
    void trimsSurroundingWhitespace() {
        assertThat(TableIdentifier.quote("  public.keepalive_ping  ")).isEqualTo("\"public\".\"keepalive_ping\"");
    }

    @ParameterizedTest
    @ValueSource(strings = { "users; drop table secrets", "users--", "public.users, other", "my table",
            "\"users\"", "users)", "1users", "public.users.extra", "", "   ", "'users'" })
    void rejectsAnythingThatIsNotAPlainIdentifier(String table) {
        assertThatIllegalArgumentException().isThrownBy(() -> TableIdentifier.quote(table));
    }

    @Test
    void rejectsNull() {
        assertThatIllegalArgumentException().isThrownBy(() -> TableIdentifier.quote(null));
    }
}
