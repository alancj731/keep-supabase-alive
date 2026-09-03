# ---- build ----------------------------------------------------------------
FROM golang:1.26-alpine AS build
WORKDIR /src

# Warm the module cache before copying sources.
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY internal/ internal/
# Static binary so the runtime image needs no libc.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/supabase-keepalive .

# ---- run ------------------------------------------------------------------
FROM alpine:3.22
WORKDIR /app

# tzdata so KEEPALIVE_TIMEZONE resolves; wget (busybox) backs the compose health check.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S keepalive && adduser -S -G keepalive keepalive

COPY --from=build /out/supabase-keepalive /app/supabase-keepalive
USER keepalive

EXPOSE 8088
ENTRYPOINT ["/app/supabase-keepalive"]
