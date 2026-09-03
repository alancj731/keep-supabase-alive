# supabase-keepalive

Supabase pauses free-tier projects after about seven days without database activity, and bringing
one back is a manual click. This service keeps any number of projects awake: on a schedule it
connects to each one and runs `select * from <table> limit 1`. The result is thrown away — the
point is the activity.

Projects are configured entirely in `.env`, comma-separated, so adding one is a one-line change.

## Quick start

```bash
cp .env.example .env      # then fill in SUPABASE_URLS and SUPABASE_TABLES
docker compose up -d --build
curl localhost:8088/api/keepalive/status | jq
```

For development, `./startdev` picks a JDK 21+ (this machine's default `java` is a JRE),
bootstraps `.env` from `.env.example` on first run, and starts the app:

```bash
./startdev                                   # run with .env from the project root
./startdev --keepalive.run-on-startup=false  # extra args go to the application
./startdev --keepalive.cron=-                # ...such as disabling the schedule
```

Or plainly, without Docker:

```bash
./mvnw package
java -jar target/supabase-keepalive-1.0.0.jar
```

`.env` is read from the working directory. Set `DOTENV_PATH=/path/to/file` (environment variable
or `-DDOTENV_PATH=`) to load it from somewhere else. Real environment variables always win over
`.env`, so Docker Compose or a PaaS dashboard can override any value.

Requires a JDK 21 or newer to build.

## Configuration

Every setting is an environment variable; see `.env.example` for the annotated version.

| Variable | Default | Meaning |
|---|---|---|
| `SUPABASE_URLS` | — | **Required.** Comma-separated connection strings, one per project |
| `SUPABASE_TABLES` | — | **Required.** One table for all projects, or one per project in URL order |
| `KEEPALIVE_CRON` | `0 0 3 * * *` | Spring 6-field cron. `-` disables the schedule |
| `KEEPALIVE_TIMEZONE` | `UTC` | Zone the cron is evaluated in |
| `KEEPALIVE_RUN_ON_STARTUP` | `true` | Ping every project once at startup |
| `KEEPALIVE_RETRY_ATTEMPTS` | `3` | Attempts per project before it counts as failed |
| `KEEPALIVE_RETRY_BACKOFF_MS` | `2000` | Linear backoff: attempt *n* waits *n* × this |
| `KEEPALIVE_CONNECT_TIMEOUT_SECONDS` | `10` | JDBC connect/login timeout |
| `KEEPALIVE_QUERY_TIMEOUT_SECONDS` | `10` | Statement query timeout |
| `KEEPALIVE_API_TOKEN` | *(empty)* | When set, `/api/**` requires `Authorization: Bearer <token>` |
| `SERVER_PORT` | `8088` | HTTP port |
| `KEEPALIVE_LOG_LEVEL` | `INFO` | `DEBUG` also logs each connection attempt and the SQL being run |
| `MANAGEMENT_HEALTH_SHOW_DETAILS` | `always` | Set to `never` to hide per-project detail from `/actuator/health` |

### Getting a connection string

Supabase dashboard → **Project Settings → Database → Connection string → URI**. Prefer the
**connection pooler** host: the direct `db.<ref>.supabase.co` host is IPv6-only on many networks.

The password is part of the URL, so percent-encode anything that would confuse a URI:

| Character | Encode as |
|---|---|
| `@` | `%40` |
| `:` | `%3A` |
| `/` | `%2F` |
| `?` | `%3F` |
| `#` | `%23` |
| `,` | `%2C` |

`sslmode=require` is added automatically when the URL does not already specify one. A
`jdbc:postgresql://…?user=…&password=…` URL is also accepted.

### Tables

`SUPABASE_TABLES` takes either a single name applied to every project, or exactly one per URL in
the same order. Names accept `table` or `schema.table`, are validated against a strict identifier
pattern, and are then double-quoted — so they are matched **case-sensitively**. Write the name
exactly as the table was created (Supabase tables are usually lowercase).

If a project has no table that is safe to read, create a tiny one:

```sql
create table public.keepalive_ping (id int primary key generated always as identity);
```

The service connects as the database user in the URL, so row-level security does not affect it.

## Endpoints

| Endpoint | Purpose |
|---|---|
| `GET /api/keepalive/status` | Per-project last result, the schedule, and the next run time |
| `POST /api/keepalive/run` | Run now, synchronously. `409` if a run is already in flight |
| `GET /actuator/health` | `DOWN` when any project's last query failed |
| `GET /actuator/health/liveness` | Is the service itself alive — use this for container health checks |

`/actuator/**` is never behind `KEEPALIVE_API_TOKEN`, so platform probes keep working.

```console
$ curl -s localhost:8088/api/keepalive/status | jq '.projects[0]'
{
  "projectId": "p1",
  "projectName": "postgres.abcdefgh@aws-0-us-east-1.pooler.supabase.com/postgres",
  "host": "aws-0-us-east-1.pooler.supabase.com",
  "port": 5432,
  "database": "postgres",
  "table": "public.keepalive_ping",
  "success": true,
  "attempts": 1,
  "durationMs": 267,
  "rowsSeen": 1,
  "error": null,
  "checkedAt": "2026-09-03T16:30:19.409332070Z"
}
```

### Seeing what it does

At the default `INFO` level each project reports one line per run:

```
Keep-alive OK: postgres.abcdefgh@aws-0-us-east-1.pooler.supabase.com/postgres table public.allowed_emails (1 row(s), 716 ms, attempt 1/3)
Keep-alive run (startup) finished: 2/2 project(s) OK in 812 ms
```

`1 row(s)` is the proof the query reached the database and came back with data; an empty table
reports `0 row(s)`, which is still a success. For the connection itself, set `KEEPALIVE_LOG_LEVEL=DEBUG`
(or pass `--logging.level.com.jian.supabasekeepalive=DEBUG`):

```
Connecting to jdbc:postgresql://aws-0-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require as postgres.abcdefgh (attempt 1/3)
Connected to postgres.abcdefgh@aws-0-us-east-1.pooler.supabase.com/postgres; running: select * from "public"."allowed_emails" limit 1
```

The logged JDBC URL never contains the password — credentials are passed as connection properties.

## How it works

- **Fail fast.** `SUPABASE_URLS` and `SUPABASE_TABLES` are parsed and validated at startup. A bad
  entry stops the service from booting, and the message names the entry number and which `.env`
  file was read — never the connection string itself, which contains the password.
- **No connection pool.** A pool would hold idle connections against every project all day for the
  sake of one query. Each ping opens a `DriverManager` connection and closes it.
- **Parallel.** Projects are pinged concurrently on virtual threads.
- **Credentials stay out of the logs.** The password is passed as a connection property, never in
  the JDBC URL, and is scrubbed from any driver error message before it is logged or returned.
- **In-memory state.** `/api/keepalive/status` reflects the current process only; a restart clears
  the history. Nothing is persisted.
- **Container health.** `/actuator/health` goes `DOWN` when a project fails, which is what you want
  for alerting, so Compose health-checks `/actuator/health/liveness` instead — an unreachable
  Supabase project must not restart the service.

Note that a completely blackholed host can take longer than
`KEEPALIVE_CONNECT_TIMEOUT_SECONDS` to give up, because the PostgreSQL driver applies that timeout
per socket attempt. Each project runs on its own thread, so a straggler only delays that project.

## Tests

```bash
./mvnw test
```

The suite covers URL parsing (including percent-encoded passwords and credential leakage), table
identifier validation and SQL-injection rejection, `.env` parsing and precedence, project/table
pairing, the retry and redaction behaviour with a stubbed JDBC layer, the health indicator, the
token filter, and the HTTP endpoints. No database or network is needed.
