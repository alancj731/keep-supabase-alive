package com.jian.supabasekeepalive.web;

import jakarta.servlet.FilterChain;
import org.junit.jupiter.api.Test;

import org.springframework.http.HttpHeaders;
import org.springframework.mock.web.MockFilterChain;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;

class ApiTokenFilterTest {

    private final ApiTokenFilter filter = new ApiTokenFilter("s3cret-token");

    @Test
    void passesThroughARequestCarryingTheToken() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/keepalive/status");
        request.addHeader(HttpHeaders.AUTHORIZATION, "Bearer s3cret-token");
        MockHttpServletResponse response = new MockHttpServletResponse();
        MockFilterChain chain = new MockFilterChain();

        this.filter.doFilter(request, response, chain);

        assertThat(response.getStatus()).isEqualTo(200);
        assertThat(chain.getRequest()).isSameAs(request);
    }

    @Test
    void acceptsALowercaseBearerScheme() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/keepalive/status");
        request.addHeader(HttpHeaders.AUTHORIZATION, "bearer s3cret-token");
        MockHttpServletResponse response = new MockHttpServletResponse();

        this.filter.doFilter(request, response, new MockFilterChain());

        assertThat(response.getStatus()).isEqualTo(200);
    }

    @Test
    void rejectsAMissingHeader() throws Exception {
        MockHttpServletResponse response = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);

        this.filter.doFilter(new MockHttpServletRequest("POST", "/api/keepalive/run"), response, chain);

        assertThat(response.getStatus()).isEqualTo(401);
        assertThat(response.getContentAsString()).isEqualTo("{\"error\":\"unauthorized\"}");
        verifyNoInteractions(chain);
    }

    @Test
    void rejectsTheWrongToken() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest("POST", "/api/keepalive/run");
        request.addHeader(HttpHeaders.AUTHORIZATION, "Bearer nope");
        MockHttpServletResponse response = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);

        this.filter.doFilter(request, response, chain);

        assertThat(response.getStatus()).isEqualTo(401);
        verifyNoInteractions(chain);
    }
}
