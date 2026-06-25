# Hybreed API

Go backend for **[Hybreed](../hybreed)** — the hybrid-athlete fitness tracker
(running + lifting + nutrition). It powers the four tabs of the Expo app — **Today**,
**Train**, **Fuel**, **You** — plus the auth flow (email/password, 6-digit OTP
verification, social sign-in) and the overlay screens (live run, lift session,
food log, activity detail).

## Stack

| Concern        | Choice                                                            |
| -------------- | ----------------------------------------------------------------- |
| Language       | Go 1.25                                                           |
| Router         | [chi](https://github.com/go-chi/chi) (net/http-native)           |
| Database       | PostgreSQL 16 via [pgx v5](https://github.com/jackc/pgx)          |
| Queries        | [sqlc](https://sqlc.dev) — type-safe code generated from raw SQL  |
| Migrations     | [golang-migrate](https://github.com/golang-migrate/migrate), embedded + auto-run |
| Cache          | Redis 7 via [go-redis](https://github.com/redis/go-redis)        |
| Auth           | bcrypt passwords · SHA-256-hashed OTP · JWT access + rotating refresh tokens |
| Config         | env-driven ([caarlos0/env](https://github.com/caarlos0/env) + `.env`) |
| Logging        | stdlib `log/slog` (text in dev, JSON in prod)                    |
| CI             | GitHub Actions — lint, sqlc-verify, test, build, Docker → GHCR    |

## Quickstart (Docker)

```bash
cp .env.example .env          # optional for compose; values are baked in
make up                       # postgres + redis + api (migrations auto-run)
make docker-seed              # load the demo athlete + food database
curl localhost:8080/healthz
```

Then sign in with the seeded account: **`sam@hybreed.app` / `trainhard`**.

## Quickstart (local)

```bash
# 1. start just the datastores
docker compose up -d postgres redis

# 2. configure + run
cp .env.example .env          # edit DATABASE_URL/REDIS_URL/JWT_SECRET if needed
make run                      # API on :8080, migrations auto-applied on boot
make seed                     # demo data
```

Requires Go 1.25+. Install the dev tools once:

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Configuration

All via environment (see [`.env.example`](.env.example)):

| Var | Default | Notes |
| --- | --- | --- |
| `ENV` | `development` | `production` switches logs to JSON and stops logging OTP codes |
| `HTTP_ADDR` | `:8080` | listen address |
| `DATABASE_URL` | — (required) | `postgres://…` |
| `AUTO_MIGRATE` | `true` | run embedded migrations on startup |
| `REDIS_URL` | `redis://localhost:6379/0` | |
| `JWT_SECRET` | — (required) | HMAC secret for access tokens |
| `ACCESS_TOKEN_TTL` | `15m` | |
| `REFRESH_TOKEN_TTL` | `720h` | 30 days |
| `OTP_TTL` | `10m` | |
| `OTP_MAX_ATTEMPTS` | `5` | |
| `CORS_ALLOWED_ORIGINS` | `*` | comma-separated |

## API

Base path: `/v1`. All responses are JSON. Errors use a stable envelope:

```json
{ "error": { "code": "unauthorized", "message": "invalid or expired token" } }
```

Protected routes require `Authorization: Bearer <accessToken>`.

### Auth — `/v1/auth`
| Method | Path | Body | Purpose |
| --- | --- | --- | --- |
| POST | `/register` | `{name,email,password}` | create account, send OTP |
| POST | `/verify` | `{email,code}` | confirm email → returns session |
| POST | `/resend` | `{email}` | re-send the OTP |
| POST | `/login` | `{email,password}` | returns session (403 `email_not_verified` if pending) |
| POST | `/refresh` | `{refreshToken}` | rotate tokens (old refresh is revoked) |
| POST | `/logout` | `{refreshToken}` | revoke a refresh token |
| POST | `/social` | `{provider,email?,name?}` | Apple/Google sign-in (**stubbed**) |
| GET | `/me` | — | current account |

### Today — `/v1/home`
- `GET /home/today` — aggregate of load ring + nutrition brief + plan + unified timeline.

### Train — `/v1/activities`, `/v1/training`
| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/activities?kind=&limit=&offset=` | history (run/lift summaries) |
| POST | `/activities` | log a run or lift (with splits / exercises+sets) |
| GET | `/activities/{id}` | full detail (run splits / lift breakdown) |
| DELETE | `/activities/{id}` | remove |
| GET | `/training/load` | Today load ring (today/weekly load, balance, 7-day chart, rest) |
| GET | `/training/plan?date=` | the day's plan |
| POST | `/training/plan` | add a plan item |
| PATCH | `/training/plan/{id}` | `{done}` toggle |
| DELETE | `/training/plan/{id}` | remove |

### Fuel — `/v1/nutrition`, `/v1/foods`
| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/nutrition/summary?date=` | calorie budget, macros, water, meals |
| POST | `/nutrition/water?date=` | `{ml}` add/subtract water |
| POST | `/nutrition/meals` | log a meal (+ inline items) |
| POST | `/nutrition/meals/{id}/items` | add a food item |
| DELETE | `/nutrition/meals/{id}` | remove a meal |
| GET | `/foods?q=&limit=` | search the food DB (**Redis-cached**) |
| GET | `/foods/barcode/{code}` | barcode lookup (**Redis-cached**) |
| POST | `/foods/estimate` | AI photo plate estimate (**stubbed**) |

### You — `/v1/me`
| Method | Path | Purpose |
| --- | --- | --- |
| GET / PATCH | `/me/profile` | identity (name, handle, status, load target, body weight) |
| GET / PATCH | `/me/settings` | units, notifications, connected apps, body metrics |
| GET | `/me/stats` | month-to-date totals + 6-week load trend |
| GET / POST | `/me/prs` | personal records |
| DELETE | `/me/prs/{id}` | remove a PR |

### Ops
- `GET /healthz` — liveness. `GET /readyz` — readiness (pings Postgres + Redis).

## How it maps to the app

The DTOs intentionally mirror `hybreed/src/data/hybreed.ts`: pace renders as
`"4:35"`, durations as `"28:30"`, macros as `{p,c,f}`, the load ring carries
`todayLoad / weeklyLoad / balance / loadWeek / rest`, and the seed reproduces
`ATHLETE`, `NUTRITION`, `TIMELINE`, `PLAN`, `HISTORY` and the `FOOD_DB`. Wiring
the app's mock screens to live data is mostly swapping the constants for `fetch`.

## Development

```bash
make ci             # fmt-check + vet + lint + test  (run before pushing)
make sqlc           # regenerate internal/store after editing db/queries/*.sql
make migrate-create name=add_widgets
make test
```

The data layer is generated: edit SQL in [`db/queries`](db/queries) and
[`db/migrations`](db/migrations), then `make sqlc`. CI fails if the committed
`internal/store` is stale (`sqlc diff`).

## Project layout

```
cmd/
  api/            entrypoint: config → pool → migrate → redis → wire → serve (graceful shutdown)
  seed/           idempotent demo-data seeder
db/
  migrations/     golang-migrate up/down (embedded into the binary)
  queries/        sqlc source queries
internal/
  config/         env-driven configuration
  database/       pgx pool + embedded migrations
  cache/          tolerant JSON-over-Redis helper
  store/          sqlc-generated, type-safe DB layer (+ pgtype convert helpers)
  httpx/          JSON envelope, typed API errors, auth-identity context
  format/         pace/clock/km display helpers (mirror the app)
  auth/           bcrypt · OTP · JWT · middleware · handlers
  athlete/        profile, settings, PRs, stats        (You)
  training/       activities, plan, load summary         (Train + Today ring)
  nutrition/      daily summary, meals, foods            (Fuel)
  home/           Today aggregation + unified timeline   (Today)
  api/            router, middleware, health probes
```
