# supabase-keepalive

Supabase pauses free-tier projects after about seven days without database activity, and bringing
one back is a manual click. This service keeps any number of projects awake: on a schedule it
connects to each one and runs `select * from <table> limit 1`. The result is thrown away — the
point is the activity.

Projects are configured entirely in `.env`, comma-separated, so adding one is a one-line change.

Go, no framework: a ~32 MB container image and a single static binary.

## Quick start

```bash
cp .env.example .env      # then fill in SUPABASE_URLS and SUPABASE_TABLES
docker compose up -d --build
curl localhost:8088/api/keepalive/status | jq
```

For development, `./startdev` checks the toolchain, bootstraps `.env` from `.env.example` on first
run, and starts the service:

```bash
./startdev                                  # run with .env from the project root
./startdev --help                           # extra args go to the application
```

Or plainly:

```bash
go build -o supabase-keepalive ./cmd/supabase-keepalive
./supabase-keepalive
```

`.env` is read from the working directory. Set `DOTENV_PATH=/path/to/file` to load it from
elsewhere. Real environment variables always win over `.env`, so Docker Compose or a hosting
platform can override any value.

Requires Go 1.23+ to build.

### Deploying to Vercel

Vercel runs functions, not long-running processes, so the in-process schedule cannot be used
there. `api/keepalive.go` is a serverless entry point that pings every project once per
invocation, and **Vercel Cron** calls it on the schedule in `vercel.json`.

```bash
vercel deploy --prod          # or connect the GitHub repo in the dashboard
```

The standalone server lives in `cmd/supabase-keepalive`, deliberately **not** at the repository
root: a root `main` package makes Vercel's Go builder compile that as a standalone server instead
of building the `api/` function.

Set these in Project → Settings → Environment Variables (there is no `.env` on Vercel):

| Variable | Value |
|---|---|
| `SUPABASE_URLS` | your comma-separated list, unchanged |
| `SUPABASE_TABLES` | unchanged |
| `CRON_SECRET` | a random string — Vercel sends it as `Authorization: Bearer …`, and the function rejects anything else |
| `KEEPALIVE_RETRY_ATTEMPTS` | `2` — keeps a run inside the function time limit |
| `KEEPALIVE_RETRY_BACKOFF_MS` | `500` |
| `GOFLAGS` | `-buildvcs=false` — Go cannot stamp VCS metadata in Vercel's build sandbox, which fails the build without this |

Things that differ from running the binary:

- **The schedule lives in `vercel.json`**, in standard 5-field cron (`0 3 * * *`).
  `KEEPALIVE_CRON` is ignored on Vercel.
- **There is no status history.** Every invocation is cold, so `/api/keepalive/status` cannot
  exist; the function returns the result of the run it just performed, and failures show up as
  a `503` in the Vercel dashboard.
- **Hobby plans currently limit cron to about one run per day** at an approximate time. That is
  ample — Supabase's window is around seven days.
- **Use the transaction pooler (port 6543)** rather than session mode: serverless functions open
  and drop connections constantly.
- If a run needs longer than the default function timeout, raise it with a `functions` entry in
  `vercel.json` (`maxDuration`), or lower the retry settings above.

`main.go`, `./startdev` and the container image are unaffected — the same code deploys either way.

## Configuration

Every setting is an environment variable; see `.env.example` for the annotated version.

| Variable | Default | Meaning |
|---|---|---|
| `SUPABASE_URLS` | — | **Required.** Comma-separated connection strings, one per project |
| `SUPABASE_TABLES` | — | **Required.** One table for all projects, or one per project in URL order |
| `KEEPALIVE_CRON` | `0 0 3 * * *` | 6-field cron (second minute hour day month weekday). `-` disables the schedule |
| `KEEPALIVE_TIMEZONE` | `UTC` | Zone the cron is evaluated in |
| `KEEPALIVE_RUN_ON_STARTUP` | `true` | Ping every project once at startup |
| `KEEPALIVE_RETRY_ATTEMPTS` | `3` | Attempts per project before it counts as failed |
| `KEEPALIVE_RETRY_BACKOFF_MS` | `2000` | Linear backoff: attempt *n* waits *n* × this |
| `KEEPALIVE_CONNECT_TIMEOUT_SECONDS` | `10` | Connect timeout |
| `KEEPALIVE_QUERY_TIMEOUT_SECONDS` | `10` | Query timeout |
| `KEEPALIVE_API_TOKEN` | *(empty)* | When set, `/api/**` requires `Authorization: Bearer <token>` |
| `KEEPALIVE_LOG_LEVEL` | `INFO` | `DEBUG` also logs each connection attempt and the SQL being run |
| `SERVER_PORT` | `PORT`, else `8088` | HTTP port. Platforms that inject `PORT` (Fly, Render, Railway) work without configuration |
| `MANAGEMENT_HEALTH_SHOW_DETAILS` | `always` | Set to `never` to hide per-project detail from the health endpoint |

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

`sslmode=require` is added automatically when the URL does not already specify one. The
`jdbc:postgresql://…?user=…&password=…` form is also accepted.

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
| `GET /actuator/health` | `503` and `DOWN` when any project's last query failed |
| `GET /actuator/health/liveness` | Is the service itself alive — use this for container health checks |

`/actuator/**` is never behind `KEEPALIVE_API_TOKEN`, so platform probes keep working. The paths
are inherited from the Spring Boot version this replaced, so existing health checks keep working;
the health body is `{"status", "details"}` rather than Spring's `components` shape.

```console
$ curl -s localhost:8088/api/keepalive/status | jq '.projects[0]'
{
  "projectId": "p1",
  "projectName": "postgres.abcdefgh@aws-0-us-east-1.pooler.supabase.com/postgres",
  "host": "aws-0-us-east-1.pooler.supabase.com",
  "port": 5432,
  "database": "postgres",
  "table": "public.allowed_emails",
  "success": true,
  "attempts": 1,
  "durationMs": 716,
  "rowsSeen": 1,
  "error": null,
  "checkedAt": "2026-09-03T18:14:22.758145652Z"
}
```

### Seeing what it does

At `INFO` each project reports one line per run:

```
level=INFO msg="Keep-alive OK" project=postgres.abcdefgh@aws-0-us-east-1.pooler.supabase.com/postgres table=public.allowed_emails rows=1 durationMs=716 attempt=1 of=3
level=INFO msg="Keep-alive run finished" trigger=startup ok=2 total=2 durationMs=812
```

`rows=1` is the proof the query reached the database and came back with data; an empty table
reports `rows=0`, which is still a success. For the connection itself, set `KEEPALIVE_LOG_LEVEL=DEBUG`:

```
level=DEBUG msg="Connecting to project" dsn="postgresql://aws-0-us-east-1.pooler.supabase.com:5432/postgres?sslmode=require" user=postgres.abcdefgh attempt=1 of=3
level=DEBUG msg=Connected project=postgres.abcdefgh@... sql="select * from \"public\".\"allowed_emails\" limit 1"
```

The logged DSN never contains the password — credentials are passed as connection parameters.

## How it works

- **Fail fast.** `SUPABASE_URLS` and `SUPABASE_TABLES` are parsed and validated at startup. A bad
  entry stops the service from booting, and the message names the entry number and which `.env`
  file was read — never the connection string itself, which contains the password.
- **No connection pool.** A pool would hold idle connections against every project all day for the
  sake of one query. Each ping opens a connection and closes it.
- **Parallel.** Projects are pinged concurrently, one goroutine each.
- **Credentials stay out of the logs.** The password is passed as a connection parameter, never in
  the logged DSN, and is scrubbed from any driver error before it is logged or returned.
- **In-memory state.** `/api/keepalive/status` reflects the current process only; a restart clears
  the history. Nothing is persisted.
- **Container health.** The health endpoint goes `DOWN` when a project fails, which is what you
  want for alerting, so Compose health-checks `/actuator/health/liveness` instead — an unreachable
  Supabase project must not restart the service.

## Tests

```bash
go test ./...
```

66 cases covering the Vercel entry point (auth, and that a bad connection string is reported
without being echoed), connection-string parsing (percent-encoded passwords, credentials kept out of the
DSN), table identifier validation and SQL-injection rejection, `.env` parsing and precedence,
config defaults and bad values, project/table pairing, the retry and redaction behaviour against a
stub connector, the concurrent-run guard, and every HTTP endpoint including the token filter and
health status codes. No database or network is needed.
