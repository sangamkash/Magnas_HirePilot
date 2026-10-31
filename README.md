# Magnas Hiring Pilot

Magnas Hiring Pilot currently includes a Vite/React frontend and a Go backend foundation. Profile records are stored in PostgreSQL through authenticated APIs. The frontend keeps job search configuration embedded in profile JSONB; job discovery providers are not connected yet.

## Run the full stack

Copy `.env.example` to `.env` and replace `AUTH_SECRET` with a unique random secret of at least 32 characters. Set `COOKIE_SECURE=true` when serving through HTTPS. Then start PostgreSQL, the backend, and the frontend:

```sh
make up
```

`make up` starts the existing Compose images. Use `make rebuild` for a clean frontend dependency install and production build, no-cache Docker image builds, and recreated services. `make compose-build` builds Compose images without starting them. `make down` stops services and preserves the named PostgreSQL volume.

Open the frontend at `http://localhost:3000`. The backend is available at `http://localhost:8080`.
Stop the services with `make down`. This preserves the named PostgreSQL data volume.

## Frontend development

```sh
make install
npm run dev
make build
```

## Backend development

Requirements: Docker Compose. The Compose setup runs the Go backend and PostgreSQL; migrations run only when explicitly requested, as a one-off command using the same backend image and Go executable. The migration commands are wrappers around the backend's Cobra CLI and custom Go SQL migration runner.

Start PostgreSQL and the backend (or use the full-stack command above):

```sh
make up
```

Apply all pending migrations (applied versions are recorded in PostgreSQL and not rerun):

```sh
make migrate-up
```

The new account flow uses `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, and authenticated `GET /api/v1/auth/me`. The backend signs 12-hour sessions into an HTTP-only, SameSite=Lax cookie. Registration accepts usernames of 3–32 letters, digits, or underscores, and passwords of 12–72 bytes. Existing UI routes (`/` and `/jobs`) require login; `/login` and `/register` are public. Frontend requests use same-origin `/api` routing through Nginx in Compose and Vite's development proxy locally.

The `000003_usernames` migration adds a case-insensitive unique username, keeps existing email values but makes email optional for username-only accounts, and preserves existing user rows. Apply migrations before registering accounts.

The backend executable provides the Cobra commands `migrate up`, `migrate down`, `migrate status`, and `migrate create <name>`. From the project root, use `make migrate-up`, `make migrate-status`, `make migrate-down`, or `make migrate-create name=add_candidate_notes`. Set `COUNT=n` to roll back the latest n migrations, for example `make migrate-down COUNT=2`. Rollback can remove application data if a migration's down SQL drops tables, so review it before running.

The backend listens on `http://localhost:8080`. Check process health and database readiness:

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
```

`GET /health` reports HTTP process health. `GET /ready` checks PostgreSQL connectivity and returns `503` until PostgreSQL is reachable; apply migrations before expecting readiness to succeed. Neither endpoint exposes database errors.

The database uses a named Docker volume and keeps its data across restarts. Stop the services with `make down`; this does not remove the volume.

Profile management uses authenticated `GET/POST /api/v1/profiles`, `GET/PUT/DELETE /api/v1/profiles/{id}`, `PUT /api/v1/profiles/{id}/activate`, and `GET /api/v1/profiles/active` endpoints. Migration `000004_profile_management` backfills existing relational profile and search data into JSONB, and profile writes transactionally keep the relational representation synchronized. The backend checks that the required profile schema exists before serving. Existing browser profile data is imported with per-profile idempotency keys and remains in localStorage until every profile is confirmed. Migration 000004 is forward-only after profile data, imports, active selections, or multiple profiles exist; review its guarded down migration before considering rollback.

Migrations are kept in `backend/migrations` and are not run automatically by the application or Docker startup. Create an empty up/down pair with `make migrate-create name=add_candidate_notes`; edit the generated files before applying them. Files use `NNNNNN_name.up.sql` and `NNNNNN_name.down.sql`; versions run in ascending order and rollback in descending order. Normal SQL and its tracking update run in a transaction. For PostgreSQL statements that cannot run in a transaction, put `-- migrate:no-transaction` in the SQL file; the SQL then executes without an encompassing transaction and migration bookkeeping follows separately, so such migrations should be written to tolerate recovery after partial execution.

To run the backend or migration commands directly on the host, install Go 1.25 or newer and set `DATABASE_URL` to a local PostgreSQL URL (or configure the supported `PG*` variables). Run `cd backend && go run ./cmd/api` to start the server, or `go run ./cmd/api migrate up`, `go run ./cmd/api migrate down`, `go run ./cmd/api migrate status`, and `go run ./cmd/api migrate create add_candidate_notes`. The server defaults to `:8080`; set `HTTP_ADDR` to change it. Set `MIGRATIONS_DIR` to override the default `migrations` directory.

Run backend verification with:

```sh
go test ./...
go vet ./...
```

Run these from `backend/`, or use `make test` from the root.

## Current scope

The backend is a modular monolith with configuration, database pooling, HTTP handlers, and versioned PostgreSQL schema migrations. Profile, authentication, search configuration, job discovery, and job match APIs are future work. No job-provider integration or fake job data is included.
