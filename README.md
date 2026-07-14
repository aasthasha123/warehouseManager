# Cold Storage Platform — Backend

A Go backend for a multi-tenant cold storage platform:

- **Company** — one cold storage business (multi-tenant: each company's data is isolated)
- **Admin (Owner)** — signs the company up, can add Supervisors
- **Supervisor (In-charge)** — onboards Farmers and logs Lots
- **Farmer (Customer)** — onboarded once, identified afterwards purely by a QR code — no password

Every scan of a farmer's QR resolves to their record and lets a Supervisor **append** a new Lot
(Quantity, Rack No., Item Name, Quality Grade) — nothing is ever overwritten, so a farmer's Lot
list is a full history.

Data is stored in **Postgres** (tested against Render Postgres). The frontend
(`cmd/server/web/index.html`) is embedded into the binary and served alongside the API, so one
binary is the whole deployable.

## Run it locally

You need a Postgres instance reachable from wherever this runs — either a local one or a hosted
one like Render's.

```bash
export DATABASE_URL="postgres://user:password@host:5432/dbname?sslmode=disable"  # local
# or, for Render (sslmode=require, not disable — see "Deploying to Render" below)
go run ./cmd/server
# -> http://localhost:8080          the frontend
# -> http://localhost:8080/api/...  the API
```

The schema is applied automatically on startup (`internal/store/schema.sql`, run via
`CREATE TABLE IF NOT EXISTS ...`) — there's no separate migration step to run by hand.

Env vars:
- `DATABASE_URL` — **required.** Standard Postgres connection string.
- `ADDR` — listen address, default `:8080`

### Local Postgres via Docker, if you don't already have one running

```bash
docker run --name coldstorage-pg -e POSTGRES_PASSWORD=devpass -e POSTGRES_DB=coldstorage \
  -p 5432:5432 -d postgres:16
export DATABASE_URL="postgres://postgres:devpass@localhost:5432/coldstorage?sslmode=disable"
go run ./cmd/server
```

## Running the whole app in Docker

The included `Dockerfile` is a multi-stage build: it compiles a fully static binary
(`CGO_ENABLED=0` — `lib/pq` is pure Go, so this works with no libc dependency) in a `golang:1.22-alpine`
stage, then copies just that binary into a minimal `alpine:3.20` runtime image. The frontend is
already embedded into the binary at compile time, so the final image is just one executable plus
CA certificates.

```bash
docker build -t coldstorage-server .
docker run -p 8080:8080 -e DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=require" coldstorage-server
```

Or, for local dev, `docker-compose.yml` spins up Postgres *and* the app together:

```bash
docker compose up --build
# -> http://localhost:8080
```
(That compose file is local-dev-only — Render doesn't use it; see the deployment options below.)

## Deploying to Render

Render can deploy this either natively (Go buildpack) or via the Dockerfile — pick whichever you
prefer, both produce the same running app:

**Option A — native Go build:**
1. Create a **Postgres** instance on Render. Copy its **External Database URL** (or Internal, if
   your web service is also on Render — faster and free of egress charges).
2. Create a **Web Service** pointing at this repo, with:
   - Build command: `go build -o app ./cmd/server`
   - Start command: `./app`
3. Set `DATABASE_URL` on the web service to the connection string from step 1 (Render's Postgres
   URLs already include `sslmode=require` — keep that; Render requires TLS on external connections).

**Option B — Docker:**
1. Same Postgres setup as above.
2. Create a **Web Service**, choose **Environment: Docker** — Render will build and run the
   included `Dockerfile` automatically, no build/start commands needed.
3. Set `DATABASE_URL` the same way.

Either way: deploy, and on first boot the app connects and creates its own schema — nothing else
to run. Replace the name/host in `DATABASE_URL` as you mentioned, and it's live.

## Architecture

```
cmd/server/main.go        — entrypoint: connects to Postgres, embeds web/, wires the HTTP server
cmd/server/web/index.html — the frontend: login/signup, register+QR, scan+lot, records, team
internal/store/           — persistence layer: models, schema.sql, and all SQL queries
internal/authutil/        — password hashing (PBKDF2-HMAC-SHA256, stdlib only)
internal/api/              — HTTP handlers, routing, auth middleware
```

**Every DB access goes through a method on `*store.DB`** (`CreateFarmer`, `AddLot`,
`FindUserByEmail`, ...) — handlers never write SQL directly. If you need to change something
about how data is stored, `internal/store/store.go` and `schema.sql` are the only files that
should need touching.

**Why `lib/pq` and not `pgx`:** this was built in a sandboxed environment whose network only
reaches a few specific domains — not `proxy.golang.org` — so dependencies had to be fetched by
setting `GOPROXY=direct` and letting Go clone straight from GitHub. `lib/pq` is a single,
dependency-free package, which made that path reliable. `pgx` is the more actively maintained,
faster driver and is a reasonable swap if you want it — the `database/sql` interface used
throughout means the swap is mostly `sql.Open("postgres", ...)` → `sql.Open("pgx", ...)` plus
an import change; every query in `store.go` stays the same since they're all plain SQL.

## Auth model

- Admin and Supervisor log in with email + password and get a bearer session token
  (`Authorization: Bearer <token>`), valid for 7 days. Sessions live in the `sessions` table.
- Farmers never log in. Their QR code encodes an opaque random token (not their Aadhar number,
  so a lost or photographed QR doesn't leak PII) that a Supervisor's authenticated session
  resolves via `GET /api/farmers/scan/{token}`. The QR is effectively the farmer's "credential."

## Free tier

Every new company starts on `plan: "free"`, capped at `FreeTierFarmerLimit` (25) farmers
(see `internal/store/store.go`). The check-and-insert on registration runs inside a transaction
that locks the company row (`SELECT ... FOR UPDATE`), so two simultaneous registrations can't
both slip in past the limit. Registering a 26th farmer returns `402 Payment Required`.
`GET /api/company/me` reports current usage so a frontend can show a "18 / 25 farmers" nudge.
There's no billing/upgrade flow wired up — `plan` is just a column you can update to `"pro"` today.

## API reference

All bodies are JSON. All routes except `/api/health`, `/api/auth/signup`, and `/api/auth/login`
require `Authorization: Bearer <token>`.

| Method | Path | Role | Purpose |
|---|---|---|---|
| GET | `/api/health` | — | Liveness check |
| POST | `/api/auth/signup` | — | Register a new company + its Admin owner |
| POST | `/api/auth/login` | — | Log in (Admin or Supervisor) |
| POST | `/api/auth/logout` | any | Invalidate the current session token |
| GET | `/api/me` | any | Current user + company + free-tier usage (rebuild session on load) |
| GET | `/api/company/me` | any | Company info + free-tier usage |
| POST | `/api/supervisors` | admin | Add a Supervisor to the company |
| GET | `/api/supervisors` | admin | List Supervisors added under the company |
| POST | `/api/farmers` | any | Onboard a farmer → returns `qr_token` to encode in a QR |
| GET | `/api/farmers` | any | List all farmers in the company |
| GET | `/api/farmers/{id}` | any | Get one farmer + their lot history |
| GET | `/api/farmers/scan/{token}` | any | Resolve a scanned QR token → farmer + lot history |
| POST | `/api/farmers/{id}/lots` | any | Append a new lot to a farmer's record |

### Example flow

```bash
# 1. Company + Admin signs up
curl -X POST localhost:8080/api/auth/signup -d '{
  "company_name": "Everfresh Cold Storage",
  "admin_name": "Aastha Rao",
  "email": "owner@everfresh.test",
  "password": "supersecret1"
}'
# -> { "token": "...", "company": {...}, "user": {...} }

# 2. Admin adds a Supervisor
curl -X POST localhost:8080/api/supervisors -H "Authorization: Bearer $ADMIN_TOKEN" -d '{
  "name": "Ravi Supervisor", "email": "ravi@everfresh.test", "password": "supervisorpass1"
}'

# 3. Supervisor logs in
curl -X POST localhost:8080/api/auth/login -d '{
  "email": "ravi@everfresh.test", "password": "supervisorpass1"
}'
# -> { "token": "$SUP_TOKEN", ... }

# 4. Supervisor onboards a farmer — this is what triggers the printed QR
curl -X POST localhost:8080/api/farmers -H "Authorization: Bearer $SUP_TOKEN" -d '{
  "aadhar": "123456789012", "name": "Ramesh Patil", "place": "Warud, Amravati", "phone": "9876543210"
}'
# -> { "id": "frm_...", "qr_token": "9f4b4122...", ... }
# Frontend encodes qr_token into a QR image (e.g. "FARMER::9f4b4122...") and hands it to the farmer.

# 5. Later: Supervisor scans that QR
curl localhost:8080/api/farmers/scan/9f4b4122... -H "Authorization: Bearer $SUP_TOKEN"
# -> full farmer record + lot history

# 6. Supervisor logs a lot against that farmer
curl -X POST localhost:8080/api/farmers/frm_.../lots -H "Authorization: Bearer $SUP_TOKEN" -d '{
  "item_name": "Soybean", "quantity": 250, "unit": "kg", "rack_no": "R-12", "quality_grade": "A"
}'
```

Every subsequent scan-and-add repeats steps 5–6 and appends another lot — the farmer's `lots`
array just keeps growing.

## What's still worth doing before this is a real production deployment

- Replace the opaque bearer tokens with signed JWTs if you scale to multiple server instances
  behind a load balancer without sticky sessions (today it's fine either way, since sessions
  live in Postgres and any instance can validate any token).
- Add rate limiting / lockout on `/api/auth/login`.
- Lock down CORS (`internal/api/server.go`) to your real frontend origin instead of `*`.
- Add an email-verification / invite-link step for onboarding Supervisors instead of an Admin
  setting their password directly.
- Consider a proper migration tool (e.g. `golang-migrate`) once the schema needs to evolve with
  existing data in it — `CREATE TABLE IF NOT EXISTS` is fine for a schema that only grows, not
  for altering existing tables.
