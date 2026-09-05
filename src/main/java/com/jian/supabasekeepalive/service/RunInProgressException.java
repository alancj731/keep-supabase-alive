package com.jian.supabasekeepalive.service;

/** Thrown when a keep-alive run is requested while one is already in flight. */
public class RunInProgressException extends RuntimeException {

    public RunInProgressException() {
        super("a keep-alive run is already in progress");
    }
}
