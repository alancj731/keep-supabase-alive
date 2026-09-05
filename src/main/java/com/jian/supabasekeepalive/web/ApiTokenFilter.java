package com.jian.supabasekeepalive.web;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.List;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;

import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.web.filter.OncePerRequestFilter;

/**
 * Requires {@code Authorization: Bearer <token>} on the API when KEEPALIVE_API_TOKEN is set.
 * Registered only in that case, and never in front of the actuator endpoints.
 */
public class ApiTokenFilter extends OncePerRequestFilter {

    private static final String BEARER = "Bearer ";

    private final List<byte[]> expected;

    public ApiTokenFilter(String... tokens) {
        this.expected = Arrays.stream(tokens)
                .filter(token -> token != null && !token.isBlank())
                .map(String::trim)
                .map(token -> token.getBytes(StandardCharsets.UTF_8))
                .toList();
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response, FilterChain filterChain)
            throws ServletException, IOException {
        String header = request.getHeader(HttpHeaders.AUTHORIZATION);
        byte[] provided = (header != null && header.regionMatches(true, 0, BEARER, 0, BEARER.length()))
                ? header.substring(BEARER.length()).trim().getBytes(StandardCharsets.UTF_8)
                : new byte[0];
        boolean accepted = this.expected.stream().anyMatch(token -> MessageDigest.isEqual(provided, token));
        if (!accepted) {
            response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
            response.setContentType(MediaType.APPLICATION_JSON_VALUE);
            response.getWriter().write("{\"error\":\"unauthorized\"}");
            return;
        }
        filterChain.doFilter(request, response);
    }
}
