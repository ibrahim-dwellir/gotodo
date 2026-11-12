# GoTodo

A small todo app in two Go modules:

| Module | Path | What it does |
|---|---|---|
| `gotodo` | `./` | JSON API under `/api/`, SQLite storage, embedded migrations, JWT auth, Swagger UI, metrics and pprof |
| `gotodo/ui` | `./ui` | Web front end (Tailwind CSS + Alpine.js). Package `gotodo/ui/web` embeds the page; `./ui` can also run on its own and proxy to the API |

`go.work` ties the two modules together for local development; `go.mod` also has `replace gotodo/ui => ./ui`, so builds work without it.

Three commands are built from this repo:

| Command | Path | Serves |
|---|---|---|
| `gotodo` | `.` | **everything in one binary**: UI at `/`, API at `/api/`, Swagger at `/swagger/` on one port, plus the ops port |
| `api` | `./cmd/api` | API and Swagger only, plus the ops port |
| `ui` | `./ui` | UI only, proxying `/api/` and `/swagger/` to a separate API |

## Run

```sh
JWT_SECRET=change-me go run .
```

- UI: http://localhost:8080
- Swagger UI: http://localhost:8080/swagger/index.html
- Raw spec: http://localhost:8080/swagger/openapi.yaml
- Metrics and pprof: http://localhost:6060/metrics, http://localhost:6060/debug/pprof/

## Build and deploy

The UI files, migrations and Swagger assets are embedded, so the binary is all you need to copy. Build it static (the SQLite driver is pure Go, so cgo is not needed) to run on any Linux x86-64 regardless of glibc version:

```sh
CGO_ENABLED=0 go build -trimpath -o dist/gotodo .
# then on the target machine:
JWT_SECRET=change-me DB_PATH=/var/lib/gotodo/gotodo.db ./gotodo
```

For other targets set `GOOS`/`GOARCH`, e.g. `GOARCH=arm64`.

To run API and UI as separate processes instead:

```sh
JWT_SECRET=change-me go run ./cmd/api   # API on :8080, ops on :6060
go run ./ui                             # UI on :3000, proxying to the API
```

### Configuration

| Variable | Default | Used by |
|---|---|---|
| `ADDR` | `:8080` | gotodo, api |
| `DB_PATH` | `gotodo.db` | gotodo, api |
| `JWT_SECRET` | random per process (tokens die on restart) | gotodo, api |
| `TOKEN_TTL` | `24h` | gotodo, api |
| `OPS_ADDR` | `:6060` (`off` disables) | gotodo, api: `/metrics` and `/debug/pprof/*` |
| `MUTEX_PROFILE_FRACTION` | `5` (`0` disables) | gotodo, api: sample 1/n mutex contention events |
| `BLOCK_PROFILE_RATE` | `5` (`0` disables) | gotodo, api: sample one blocking event per n ns blocked |

All listen addresses bind every interface by default; use e.g. `OPS_ADDR=127.0.0.1:6060` to keep a port local.
| `UI_ADDR` | `:3000` | ui (standalone only) |
| `API_URL` | `http://localhost:8080` | ui (standalone only) |

## API

All todo and user routes need `Authorization: Bearer <token>` from `/api/auth/login`.

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/auth/register` | `first_name`, `last_name`, `email`, `password` (≥ 8 chars) → 201 user, 409 if email taken |
| `POST` | `/api/auth/login` | `email`, `password` → `{token, expires_at, user}`; stamps `last_login_at` |
| `GET` | `/api/users/me` | current user |
| `PUT` | `/api/users/me` | change `first_name` / `last_name`; bumps `updated_at` |
| `GET` | `/api/todos` | own todos, newest first; optional `?status=` filter |
| `POST` | `/api/todos` | `title` required; `description`, `status` (default `not_started`), `due_date` (`YYYY-MM-DD`) optional |
| `GET` | `/api/todos/{id}` | |
| `PUT` | `/api/todos/{id}` | full replace; `title` and `status` required; omitted `due_date` clears it |
| `DELETE` | `/api/todos/{id}` | 204 |
| `GET` | `/api/health` | |

Status values: `not_started`, `in_progress`, `in_review`, `completed`.

Every todo query is scoped by the caller's user id, so another user's todo returns 404 (existence is not leaked).

Quick try with curl:

```sh
curl -s -XPOST localhost:8080/api/auth/register \
  -d '{"first_name":"Ada","last_name":"Lovelace","email":"ada@example.com","password":"password123"}'
TOKEN=$(curl -s -XPOST localhost:8080/api/auth/login \
  -d '{"email":"ada@example.com","password":"password123"}' | jq -r .token)
curl -s -XPOST localhost:8080/api/todos -H "Authorization: Bearer $TOKEN" \
  -d '{"title":"Write report","status":"in_progress","due_date":"2026-12-01"}'
curl -s localhost:8080/api/todos -H "Authorization: Bearer $TOKEN"
```

## Metrics and profiling

The API serves a second, internal-only port (`OPS_ADDR`, default `:6060`). Keep it off the public internet; the public `:8080` port does not expose these paths.

| Path | What |
|---|---|
| `/metrics` | Prometheus metrics: `gotodo_http_requests_total{method,route,code}`, `gotodo_http_request_duration_seconds`, `gotodo_http_requests_in_flight`, `gotodo_auth_logins_total{result}`, `gotodo_auth_registrations_total{result}`, SQLite pool stats (`go_sql_*`), Go runtime and process metrics |
| `/debug/pprof/` | standard Go pprof: `profile` (CPU), `heap`, `allocs`, `goroutine`, `mutex`, `block`, `threadcreate`, `trace` |
| `/debug/pprof/delta_{heap,mutex,block}` | [godeltaprof](https://github.com/grafana/pyroscope-go/tree/main/godeltaprof) delta profiles, which only report what changed since the last pull |

`route` is the matched route pattern (`GET /api/todos/{id}`), not the raw path, so ids never become label values.

Ready-to-use configs in [deploy/](deploy):

- [deploy/otelcol-config.yaml](deploy/otelcol-config.yaml): OpenTelemetry Collector (contrib) pulls the pprof endpoints with the `pprof` receiver and pushes to Pyroscope over OTLP, tagged `service.name=gotodo`. Profiles are alpha in the collector, so start it with the feature gate:
  ```sh
  otelcol-contrib --config deploy/otelcol-config.yaml --feature-gates=service.profilesSupport
  ```
  Tested with otelcol-contrib v0.165.0 and Pyroscope v2.4.3. In Pyroscope, heap profiles from this path show up as the `space` profile type.
- [deploy/prometheus.yml](deploy/prometheus.yml): Prometheus scrape job for `localhost:6060/metrics` (tested with Prometheus v3.16.0).
- [deploy/config.alloy](deploy/config.alloy): Grafana Alloy does all of it in one agent, every 1m. It sends app metrics and host metrics (built-in node exporter) to Prometheus at `localhost:9090` by remote write, and app profiles (CPU, godeltaprof heap/mutex/block, goroutines) to Pyroscope at `localhost:4040`. Prometheus must run with `--web.enable-remote-write-receiver`. Don't run it alongside the collector config: both would compete for the CPU profile endpoint. Tested with Alloy v1.21.3.
- [deploy/grafana/gotodo-dashboard.json](deploy/grafana/gotodo-dashboard.json): Grafana dashboard for the app metrics. It covers overview stats, HTTP traffic, latency, errors and a per-route table, auth, the Go runtime, the process, and the SQLite connection pool. Import it with Dashboards → New → Import and pick your Prometheus data source. You can filter by job, instance and route. Panels use a 1m minimum interval to match the Alloy scrape interval. Tested on Grafana 13.3.1.
- [deploy/grafana/gotodo-profiles-dashboard.json](deploy/grafana/gotodo-profiles-dashboard.json): Grafana dashboard for the Pyroscope profiles. It has trend graphs (CPU, heap, allocation rate, goroutines, mutex and blocking time) and flame graphs for CPU, allocations, heap in use, goroutines, mutex and blocking. Import it the same way and pick your Pyroscope data source. It uses the profile type names that Alloy produces (`memory`, `mutex`, `block`). The OpenTelemetry Collector path names them differently, so those panels stay empty with that setup. Both dashboards link to each other.

## Data model and migrations

Migrations are plain SQL in [internal/db/migrations](internal/db/migrations), embedded in the binary and applied with [goose](https://github.com/pressly/goose) at startup. Add a new one as `0000N_description.sql` with `-- +goose Up` / `-- +goose Down` sections.

- **users**: `id`, `first_name`, `last_name`, `email` (unique, case-insensitive), `password_hash` (bcrypt), `last_login_at`, `created_at` (registered), `updated_at` (profile last modified; a login does not count)
- **todos**: `id`, `user_id` → users, `title`, `description`, `status` (CHECK-constrained), `due_date`, `created_at`, `updated_at`

## Test

```sh
go test ./... ./ui/...
```

The API tests start the real HTTP handler against a freshly migrated SQLite file per test, covering registration, login, CRUD, validation, per-user isolation, Swagger, and the metrics output. The telemetry tests check that every pprof and delta endpoint returns a parseable profile.
