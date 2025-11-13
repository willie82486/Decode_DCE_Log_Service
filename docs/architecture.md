## Decode DCE Log Service — L1 Software Architecture & Design

### 0. Document Info
- System: Decode DCE Log Service
- Version: v1.2 (Revised after build-id auto-detection)
- Date: 2025-11-13
- Level: L1 (Architecture & High-level Design)

---


### 1. Introduction & Goals
- Purpose: Provide an interface for users to upload a DCE encoded log (`dce-enc.log`) and automatically decode it. The backend extracts the GNU Build ID directly from the uploaded log via a short `hexdump` of the first 4 lines, resolves the corresponding ELF from the database, invokes `nvlog_decoder`, and returns a decoded log file.
- Goals:
  - Three services deployment: frontend (Nginx SPA + reverse proxy), backend (Go API), database (MariaDB for local; managed DB for cloud in future).
  - Operational simplicity: health checks, container log rotation, baseline security/observability.
  - Scalability: backend horizontally scalable.

#### Executive Summary
- What it does: A focused service to decode DCE logs with minimal user input. Users upload a single `dce-enc.log`; the system auto-detects the Build ID, fetches the matching decoder ELF from an internal library, runs `nvlog_decoder`, and streams back `dce-decoded.log`.
- Who uses it: Operators and engineers needing quick, reliable decoding of device logs (with admin users curating the decoder ELF library).
- How it’s exposed: A React single-page app served by Nginx that reverse-proxies all `/api` traffic to the Go backend; JWT-based authentication; MariaDB (local) or managed DB (cloud) for persistence.
- Why it’s simple: No manual Build ID entry, clear errors, health endpoints, and admin workflows to ingest/curate required ELF artifacts.

#### 1.1 Audience & Reading Guide
- Operators (day-to-day use): see Section 6 (Operations Guide & Database Schema) for commands, health checks, and troubleshooting.
- Backend developers: see Sections 3.4 (APIs), 4.1–4.2 (component internals, ELF library), and 4.5 (configuration/env vars).
- Frontend developers: see Section 4.1 (Frontend + Nginx), and `Frontend/nginx.conf` for reverse proxy behavior.
- Admin users: see Section 3.4 (Admin APIs) and 4.2 (ELF library flows: upload, by-URL, list/delete).
- SRE/Platform engineers: see Sections 4.4 (Deployment & Environments), 4.6–4.7 (Networking/Security, Health & Logging), and 6 (Ops, DDL).

---

### 2. Scope & Out of Scope
- In Scope: frontend (Nginx static + proxy), backend (Go REST API), database (MariaDB locally, cloud managed DB), containerization and health checks.
- Out of Scope: complex RBAC, workflow engines, real-time pipelines, cross-region active-active (future).

#### 2.1 Feature Map (At-a-glance)
- Log Decode: upload only `dce-enc.log` → auto-detect Build ID → decode → download `dce-decoded.log`. Adds `X-Build-Id` and `X-ELF-File` headers for traceability.
- ELF Library (Admin):
  - Upload a `.elf` manually; extract/normalize Build ID; upsert into DB.
  - Fetch by URL with live progress via SSE; automatically locate `display-t234-dce-log.elf` inside vendor archives and store it.
  - List and delete ELF records; dedup by `build_id` (latest write wins).
- Auth & Roles: `POST /api/login` issues HS256 JWT including user `role`; admin-only guards for ELF and user management.
- Health & Ops: `/nginx-health` (Nginx), `/healthz` (backend), container log rotation, clear failure messages (including decoder output on errors).

---

### 3. High-level Architecture
#### 3.1 Architecture Overview
- Request path: Client → Frontend (Nginx; serves SPA and proxies `/api`) → Go Backend (business logic) → Database.
- Containers:
  - Frontend: Nginx serves SPA and proxies `/api` to backend over the Docker network.
  - Backend: Go HTTP server (internal only in compose; exposed through frontend).
  - Database: MariaDB (local dev) or managed DB in cloud.
- Decode Flow (runtime):
  1) Frontend uploads `dce-enc.log` via `POST /api/decode` (multipart; file only).
  2) Backend saves the file to a temp workspace.
  3) Backend executes `hexdump -C dce-enc.log | head -n 4`, parses bytes at offsets `0x20..0x33` (20 bytes) into a lowercase 40-hex Build ID.
  4) Backend loads the matching ELF blob from DB by Build ID and writes it to a temp path.
  5) Backend runs `nvlog_decoder` to produce `dce-decoded.log`, then streams it back as a file download.
  6) Response headers include `X-Build-Id` and `X-ELF-File` for traceability.

#### 3.2 Tech Stack at a Glance
- Frontend: React (Vite build) served by Nginx; reverse proxy `/api` to `go-backend:8080`; health at `/nginx-health`; `client_max_body_size 512m`.
- Backend: Go `net/http`; endpoints under `/api/*`; health at `/healthz`; calls `nvlog_decoder`; reads/writes ELF blobs from/to DB.
- Database: MariaDB (local/dev via Docker Compose). Managed PostgreSQL is planned for cloud (Milestone 2).
- Artifacts: `nvlog_decoder` shipped in backend image at `/usr/local/bin/nvlog_decoder`.
- Deployment: Docker Compose for local; production compose exposes only frontend (backend internal).

#### 3.3 Current UI Behavior
- Login (role-aware) → Log Decoder page (upload `dce-enc.log`) → Download decoded log.
- Admin page:
  - Manage users.
  - Manage ELF library (details below).

#### 3.4 APIs (High-level)
- Auth
  - `POST /api/login`: body `{username,password}`; returns `{success, role, token}` (HS256; `JWT_SECRET`).
- Decode
  - `POST /api/decode` (Bearer token required): multipart upload with field `file` only.
    - Behavior: save → extract Build ID from hexdump → fetch ELF by Build ID → decode → return file.
    - Headers: `X-Build-Id`, `X-ELF-File` on success; on failure, returns clear error messages and logs decoder output.
    - Notes: legacy fields `buildId`/`pushtag` are tolerated by backend for compatibility, but the UI does not send them (prefers auto-detection).
- Admin
  - `GET /api/admin/users`, `POST /api/admin/users`, `DELETE /api/admin/users?id=<id>`
  - `GET /api/admin/elves` → `[ { buildId, elfFileName }, ... ]`
  - `DELETE /api/admin/elves?buildId=<id>`
  - `POST /api/admin/elves/upload` (multipart `elf`)
  - `POST /api/admin/elves/by-url` (JSON `{pushtag,url}`), and `GET /api/admin/elves/by-url/stream?pushtag=...&url=...` (SSE progress)
    - SSE client hint: set `Accept: text/event-stream` to receive progress events (`event: step|error|done`).
- Health
  - `GET /healthz` → 200 OK

- API Quick Reference
  | Path | Method | Auth | Description |
  |---|---|---|---|
  | `/api/login` | POST | None | Log in and issue a JWT (returns `success, role, token`). |
  | `/api/decode` | POST | Bearer | Upload `file` (`dce-enc.log`), auto-extract Build ID, find the matching ELF, run `nvlog_decoder`, and return `dce-decoded.log` (adds `X-Build-Id`, `X-ELF-File`). |
  | `/api/admin/users` | GET | Bearer (admin) | List users (passwords omitted). |
  | `/api/admin/users` | POST | Bearer (admin) | Create a user (`{username,password,role}`). |
  | `/api/admin/users?id=<id>` | DELETE | Bearer (admin) | Delete a user by `id`. |
  | `/api/admin/elves` | GET | Bearer (admin) | List ELF records (`buildId, elfFileName`). |
  | `/api/admin/elves?buildId=<id>` | DELETE | Bearer (admin) | Delete an ELF record by `buildId`. |
  | `/api/admin/elves/upload` | POST | Bearer (admin) | Upload a single `.elf` (field: `elf`), read GNU Build ID, and upsert into DB. |
  | `/api/admin/elves/by-url` | POST | Bearer (admin) | With `{pushtag,url}`, download/extract artifacts, locate `display-t234-dce-log.elf`, extract Build ID, and store in DB. |
  | `/api/admin/elves/by-url/stream?pushtag=...&url=...` | GET | Bearer (admin) | SSE progress for the by-URL flow (`step|error|done`). |
  | `/healthz` | GET | None | Backend health check (200 OK). |
  | `/nginx-health` | GET | None | Frontend (Nginx) health check (200 OK; access_log off). |
---

### 4. Service Internals
#### 4.1 Component Design
- Frontend (React + Nginx)
  - Vite for build. Nginx serves `index.html` and static assets from `/usr/share/nginx/html`.
  - Reverse proxy `/api` to `go-backend:8080` (same Docker network).
  - Health endpoint `/nginx-health` returns 200 with `access_log off`.
  - Reverse proxy hardening for long tasks (decoder, archive extraction):
    - `proxy_connect_timeout 300s`, `proxy_send_timeout 300s`, `proxy_read_timeout 600s`, `send_timeout 600s`.
    - Streaming-friendly: `proxy_request_buffering off`, `proxy_buffering off`.
  - Upload limits: `client_max_body_size 512m` (Nginx) to support large ELF/log uploads; backend currently parses multipart with a ~100MB in-memory threshold (files spill to disk).
  - UI: the Log Decoder page now only asks the user to upload `dce-enc.log` (no `buildId` or `pushtag` fields).
- Backend (Go)
  - Endpoints:
    - `POST /api/login` issues a JWT with the user role.
    - `POST /api/decode` (Bearer token required): multipart with `file` only; auto-extract Build ID from hexdump; fetch ELF by Build ID; run decoder; return `dce-decoded.log`. Adds `X-Build-Id` and `X-ELF-File` headers.
    - Admin (Bearer token; role=admin):
      - `GET/POST/DELETE /api/admin/users`
      - `GET /api/admin/elves` (list `{ buildId, elfFileName }`), `DELETE /api/admin/elves?buildId=...`
      - `POST /api/admin/elves/upload` (multipart `elf`): extract Build ID from ELF (via `readelf -n` or SHA1 fallback), store blob.
      - `POST /api/admin/elves/by-url` and `GET /api/admin/elves/by-url/stream`: download and extract artifacts by URL, locate `display-t234-dce-log.elf`, extract Build ID, and store; the `/stream` variant emits step-by-step progress via SSE.
    - `GET /healthz` (liveness/readiness).
  - Build ID extraction
    - ELF: primary via `readelf -n` Build ID; fallback to SHA1(file bytes).
    - Log: via `hexdump -C <log> | head -n 4`, concatenating 16 bytes from the `0x20` line and 4 bytes from the `0x30` line to form 40-hex Build ID.
  - Decoder invocation
    - Command: `nvlog_decoder -d none -i <encodedLog> -o <decodedLog> -e <elfPath> -f DCE`.
    - The `-e` parameter points to the actual temp ELF path (not an augmented name).
    - Robust error handling: captures `CombinedOutput`; verifies the decoded file exists and is non-empty before returning.
 
- Database
  - Local/dev DB: MariaDB
  - Cloud/prod (planned, Milestone 2): Azure Database for PostgreSQL with automated backups and HA; Private Endpoint preferred.
  - Data Model:
    - Tables:
      - `users`: `id` (PK), `username` (UNIQUE), `password` (plaintext for dev), `role`, `created_at`.
      - `build_elves`: `build_id` (PK), `elf_filename`, `elf_blob` (LONGBLOB), `created_at`.
  - Connection:
    - Local (implemented): `MYSQL_DSN=dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true`
    - Cloud (planned): `DATABASE_URL=postgres://<user>:<password>@<host>:5432/<db>?sslmode=require`


---

#### 4.2 ELF Library Management (Detailed)

This module provides a complete workflow to ingest, store, and curate the DCE decoder ELF artifacts that are used by `/api/decode`.

- Data model
  - Table: `build_elves`
    - `build_id` VARCHAR(255) PRIMARY KEY
    - `elf_filename` VARCHAR(255) (original or normalized name)
    - `elf_blob` LONGBLOB (full binary of `display-t234-dce-log.elf`)
    - `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP
  - Upsert semantics: inserts or updates existing row on the same `build_id` (latest elf blob wins).

- Admin APIs
  - `GET /api/admin/elves`
    - Returns an array of `{ buildId, elfFileName }`, sorted by `created_at DESC`.
  - `DELETE /api/admin/elves?buildId=<id>`
    - Permanently removes the record for the build ID.
  - `POST /api/admin/elves/upload` (multipart `elf`)
    - Accepts a single ELF file (field name `elf`).
    - Extracts Build ID using `readelf -n` (GNU Build ID). If unavailable, falls back to SHA1(file bytes).
    - Filename normalization rules:
      - If the original name already matches `display-t234-dce-log.elf__<pushtag>__<40-hex>`, it is preserved.
      - Otherwise, it is normalized to `display-t234-dce-log.elf__<buildId>`.
    - Stores the full binary in DB with upsert semantics.
    - Response JSON: `{ success, buildId, elfFileName }`.
  - `POST /api/admin/elves/by-url` (JSON `{pushtag,url}`) and `GET /api/admin/elves/by-url/stream?pushtag=...&url=...`
    - Download `full_linux_for_tegra.tbz2` from `<url>`, extract, locate `host_overlay_deployed.tbz2`, extract overlay, and find `display-t234-dce-log.elf`.
    - Extract Build ID (as above), read bytes, upsert into DB.
    - The `/stream` variant emits Server-Sent Events (SSE) to report progress:
      - `event: step` with human-readable messages (e.g., "Downloading...", "Extracting overlay...", "Storing ELF...").
      - `event: error` on failures (download/extract/locate/DB).
      - `event: done` with data `{"buildId": "...", "elfFileName": "..."}` on success.

      SSE Event Format and Examples:

      - Client header: `Accept: text/event-stream`
      - Event types: `step`, `error`, `done`

      Example stream (raw SSE):

      ```text
      event: step
      data: Creating temp workspace...

      event: step
      data: Downloading full_linux_for_tegra.tbz2...

      event: step
      data: Extracting main archive...

      event: step
      data: Locating host_overlay_deployed.tbz2...

      event: step
      data: Extracting overlay and locating display-t234-dce-log.elf...

      event: step
      data: Extracting Build ID and storing ELF...

      event: done
      data: {"buildId":"4f2c6c0e6b1a9b7f3f4e3d2c1b0a998877665544","elfFileName":"display-t234-dce-log.elf__r36-abc__4f2c6c0e6b1a9b7f3f4e3d2c1b0a998877665544"}
      ```

      Payload semantics:

      - `step` → `data` is a plain string message.
      - `error` → `data` is a plain string error message (client should stop consuming the stream).
      - `done` → `data` is JSON with fields:
        - `buildId` (string, 40-hex)
        - `elfFileName` (string, stored/normalized name)

- Admin UI
  - Upload ELF
    - File picker for `.elf`; upon success, shows the resolved Build ID, clears the input, and refreshes the list.
  - Fetch ELF by URL
    - Inputs: `pushtag`, `url`; show live progress via SSE and persist the progress to `localStorage` (key: `dce_by_url_state`) so a page refresh does not lose context; a "Clear" button resets the saved progress.
  - List & Delete
    - Displays `Build ID` and `ELF File Name`; provides a Delete action (with confirmation, then calls `DELETE`).

- ⚠️ **Warning:** Operational notes
  - Only users with role `admin` can access these endpoints and UI.
  - Binary size: stored in LONGBLOB; ensure DB instance/volume capacity and retention policy meet your needs.
  - Re-ingesting the same `build_id` will overwrite the previous binary and filename (upsert).
  - Decoder expects the real filesystem path for `-e`; the backend writes DB blob to a temp file and passes that absolute path.

---

#### 4.3 Container Images
- Backend
  - Build stage: `golang:1.22-alpine` (compile static binary).
  - Runtime stage: `debian:bookworm-slim` with `binutils` (for `readelf`) and `bsdextrautils` (for `hexdump`), plus `curl`, `bzip2`, `tar`.
  - Ships `nvlog_decoder` at `/usr/local/bin/nvlog_decoder`.
- Frontend
  - `nginx:alpine`. Serves built SPA and proxies `/api` to backend.

---

#### 4.4 Deployment & Environments
- Pipeline: Build images → run locally with Docker Compose; in cloud, publish to registry and deploy (e.g., Azure Container Apps).
- Frontend Container:
  - Ingress enabled (HTTP for local). Proxies `/api` to backend within the network.
- Backend Container:
  - No host port published in production-mode compose; reachable via frontend proxy.
  - Secrets: `JWT_SECRET`, `MYSQL_DSN`/`DATABASE_URL` (cloud: prefer managed DB and SSL).
- Database:
  - Local/dev: MariaDB (compose) with mounted init script and a one-shot migration container.
  - Cloud/prod: Azure Database for PostgreSQL (planned). The provided `docker-compose.prod.yml` exposes only the frontend over HTTP and expects an external managed DB.
- Scaling:
  - Frontend: scale by requests/connections; min 1 instance.
  - Backend: scale by CPU/HTTP concurrency; min 1 instance.

---

#### 4.5 Configuration & Environment Variables
- Backend
  - Listen port: fixed at `8080` (via code). No `APP_ENV`/`LOG_LEVEL` flags at present.
  - Upload handling: multipart parsed with ~100MB in-memory threshold; large files spill to disk; Nginx hard cap is 512MB.
- Auth
  - `JWT_SECRET`: HS256 signing key for JWT issuance/verification (required in non-dev).
- Database
  - Local/dev (MariaDB): `MYSQL_DSN=dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true`
  - Cloud (PostgreSQL): planned for Milestone 2 (`DATABASE_URL=postgres://...`), not implemented in current code.
- Frontend/Nginx
  - Port mapping in Compose: host `3000` -> container `80`; reverse-proxy to `go-backend:8080`.
- Config precedence
  - Environment variables > `.env` (local only) > image defaults.
- Secrets management
  - Do not bake secrets into images. For cloud, prefer ACA Secrets / Key Vault references. For local, use git-ignored `.env`.
- Security-related toggles
  - CORS is naturally same-origin via Nginx. No explicit CORS headers are set in backend.
  - Disable directory listing in Nginx; restrict upload size at Nginx and backend.
- Resource limits (recommended)
  - Set container CPU/memory limits; configure `ulimit nofile` for concurrency; enable log rotation (already configured).

---

#### 4.6 Networking & Security
- Perimeter: Frontend exposed; backend internal-only; DB internal.
- TLS: Terminate at ingress in cloud (fronting proxy/ingress). For local, HTTP-only.
- Secrets: Use environment variables; in cloud, use ACA Secrets/Key Vault.
- JWT: HS256 with `JWT_SECRET`. Authorization header is preserved by Nginx proxy.
- Least privilege: DB user limited; container logs rotated.

---

#### 4.7 Health & Logging
- Health Checks:
  - Frontend: `/nginx-health` (200).
  - Backend: `/healthz` (200).
- Logs:
  - Docker `json-file` with rotation.
  - Backend prints detailed decoder stdout/stderr on failure; includes `X-Build-Id` in responses to aid troubleshooting.

---

#### 4.8 Non-functional Requirements (NFRs)
- Availability: ≥ 99.9% business hours.
- Performance: bound by decoder and archive operations; backend horizontally scalable.
- Security: JWT, secrets management, minimal exposure, private networking in cloud.
- Operability: health probes, log rotation, structured error messages, and diagnostics.

---

#### 4.9 Risks & Mitigations
- External artifacts unavailable or slow:
  - SSE progress for by-URL fetch; explicit error propagation.
- `nvlog_decoder` compatibility/resource usage:
  - Capture outputs; validate produced file; use worker model if needed later.
- Large files/long-running tasks:
  - Future option: async jobs and status tracking UI.

---

### 5. Development Journal
#### 5.1 Roadmap

- Milestone 0 — Project Bootstrap
  - Initialize repo structure (Frontend/Backend), coding standards.
  - Add basic README and architecture docs (this file).
  - Decide how to obtain/package `nvlog_decoder` (licensing, delivery path via `NVLOG_DECODER_PATH`).
  - Draft `docker-compose.yml` with Nginx, Go backend, and MariaDB.

- Milestone 1 — Local MVP (DB-backed)
  - Backend:
    - Implement `POST /api/login` against local MariaDB (plaintext for dev; switch to hashing in cloud).
    - Implement `/api/admin/users` (GET/POST/DELETE); enforce unique username; no password in responses; admin-only middleware.
    - Implement ELF library:
      - `GET /api/admin/elves`, `DELETE /api/admin/elves?buildId=...`
      - `POST /api/admin/elves/upload` (multipart `elf`) with Build ID extraction via `readelf -n`; SHA1 fallback; upsert to `build_elves`.
      - `POST /api/admin/elves/by-url` and `GET /api/admin/elves/by-url/stream` with SSE progress to ingest artifacts by URL.
    - Implement `POST /api/decode` (file only): auto-extract Build ID via `hexdump`, fetch ELF by Build ID, run decoder, return file with `X-Build-Id` and `X-ELF-File`.
    - Provide `/healthz` health endpoint and structured error messages with decoder outputs on failure.
  - Frontend:
    - LoginPage (calls `/api/login` and stores JWT + role).
    - LogDecoder page (single file input; no `buildId`/`pushtag`; shows headers on success; downloads decoded file).
    - AdminPage:
      - Manage users.
      - Manage ELF library: upload `.elf`, fetch by URL with live SSE progress, list & delete.
    - Nginx serves SPA and reverse proxies `/api`; `/nginx-health` for liveness.
  - Database (MariaDB, local-only):
    - Compose service `mariadb:11` with:
      - `MARIADB_DATABASE=dce_logs`, `MARIADB_USER=dce_user`, `MARIADB_PASSWORD=dce_pass`, `MARIADB_ROOT_PASSWORD=rootpassword`
    - Backend DSN via env: `MYSQL_DSN=...`
    - Auto-create tables on backend startup:
      - `users(id, username UNIQUE, password, role, created_at)`
      - `build_elves(build_id PK, elf_filename, elf_blob LONGBLOB, created_at)`
    - Healthcheck: `mysqladmin ping`
    - Optional: add a named volume to persist DB data across container rebuilds.
  - Containerization:
    - Dockerfiles for frontend/backend; compose file with healthchecks and container log rotation.

- Milestone 2 — Cloud Baseline (ACA + Managed DB)
  - Build/push images to ACR; deploy two Container Apps (frontend with Ingress, backend internal-only).
  - Configure secrets via ACA (e.g., `DATABASE_URL`, `JWT_SECRET`); bind custom domain and TLS (Front Door or ACA).
  - Provision Azure Database for PostgreSQL (Flexible); Private Endpoint/VNet integration; enforce SSL.
  - Migrate schema and app to `DATABASE_URL`; switch passwords to bcrypt; add simple migrations and basic admin audit logs.

- Milestone 3 — Observability & Operations
  - Centralize logs to Log Analytics; create dashboards (decoder success/error rate, latency, Build-ID miss rate).
  - Alerts for high error rate and DB CPU/storage thresholds.
  - Backup/restore drill for DB; initial load/perf tests; cost baseline and right-sizing.

- Milestone 4 — Optional Enhancements
  - Async decode jobs (Queue + Worker).
  - Frontend job status UI.
  - Rate limiting and upload size hard caps; per-user quotas.
  - Caching/prewarming for frequently used ELFs.

- Out of Scope for Now
  - “Hard” security hardening (WAF tuning, end-to-end TLS everywhere, formal threat modeling); keep baseline security only: TLS at ingress, least privilege, secrets management, private networking preference.

---
### 6. Operations Guide & Database Schema

This section provides command-focused guidance for developers/operators to build, run, test, and debug the system locally.

#### 6.1 Command Quick Start
- Build and start all services:
  ```bash
  docker compose up -d --build
  ```
- Force rebuild backend (replace with any container you modify) only (no cache), then restart it:
  ```bash
  docker compose build --no-cache go-backend && docker compose up -d go-backend
  ```
- Tail logs quickly:
  ```bash
  docker logs dce-log-go-backend --tail 200
  docker logs dce-log-mariadb --tail 200
  docker logs dce-log-web-frontend --tail 200
  ```
- Exec into a container shell:
  ```bash
  docker compose exec dce-log-go-backend sh
  docker compose exec dce-log-mariadb bash
  ```

#### 6.2 Container Management
- List containers and health:
  ```bash
  docker ps
  docker inspect dce-log-mariadb --format '{{json .State.Health}}' | jq
  ```
- Live log streaming:
  ```bash
  docker logs -f dce-log-go-backend
  ```
  or
   ```bash
  docker compose logs -f go-backend 
   ```
  Notes:
  - `docker compose logs -f go-backend` targets the Compose service name, aggregates logs across replicas, and keeps following logs across container recreations.
  - `docker logs -f dce-log-go-backend` targets a single container by name/ID; it does not aggregate replicas and needs to be re-run if the container is recreated with a new name/ID.
  - Both support flags like `--tail`, `--since`, and `--timestamps`.
  
- Restart a single service:
  ```bash
  docker compose restart go-backend
  ```
- Stop all and remove (keep volumes):
  ```bash
  docker compose down
  ```
- Stop all and remove including volumes (DANGER: wipes DB data):
  ```bash
  docker compose down -v
  ```

#### 6.3 Health Checks
- Frontend (from host):
  ```bash
  curl -sS -I http://localhost:3000/nginx-health
  ```
- Backend (from within Docker network; e.g., run in frontend container):
  ```bash
  docker compose exec dce-log-web-frontend curl -I http://go-backend:8080/healthz
  ```
- Database health:
  ```bash
  docker inspect dce-log-mariadb --format '{{json .State.Health}}' | jq
  docker compose exec dce-log-mariadb which mariadb-admin
  ```

#### 6.4 Basic API Test Commands (Local)
- Login and get a JWT:
  ```bash
  # default seeded admin (local/dev): username=nvidia password=nvidia
  TOKEN=$(curl -s -X POST http://localhost:3000/api/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"nvidia","password":"nvidia"}' | jq -r .token)
  echo "$TOKEN"
  ```
- Admin: list users
  ```bash
  curl -s -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/admin/users | jq
  ```
- Admin: add and delete a user

  ```bash
  # add user
  curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -X POST http://localhost:3000/api/admin/users \
    -d '{"username":"test","password":"test","role":"user"}' | jq
  # Delete by id:
  curl -s -H "Authorization: Bearer $TOKEN" -X DELETE 'http://localhost:3000/api/admin/users?id=<USER_ID>' | jq
  ```
- Admin: list ELF records
  ```bash
  curl -s -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/admin/elves | jq
  ```
- Admin: upload an ELF
  ```bash
  curl -s -H "Authorization: Bearer $TOKEN" -X POST \
    http://localhost:3000/api/admin/elves/upload \
    -F elf=@./display-t234-dce-log.elf | jq
  ```
- Admin: fetch by URL (non-stream + stream)
  ```bash
  curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -X POST http://localhost:3000/api/admin/elves/by-url \
    -d '{"pushtag":"r36-abc","url":"http://.../r36-abc/latest"}' | jq

  # Stream progress (SSE); watch raw events:
  curl -N -H "Authorization: Bearer $TOKEN" \
    "http://localhost:3000/api/admin/elves/by-url/stream?pushtag=r36-abc&url=http://.../r36-abc/latest"
  ```
- Decode upload (user flow): only the file is required; Build ID is auto-detected
  ```bash
  curl -s -H "Authorization: Bearer $TOKEN" \
    -X POST http://localhost:3000/api/decode \
    -F file=@./dce-enc.log \
    -o dce-decoded.log -D headers.txt
  echo "Build-ID header:" && grep -i X-Build-Id headers.txt || true
  echo "ELF header:" && grep -i X-ELF-File headers.txt || true
  ```
  
  ---

#### 6.5 Database Schema (DDL)

This section provides concise DDLs for MariaDB (local) and PostgreSQL (cloud). They reflect the data model described in Section 4.1/4.2.
Note: PostgreSQL support is planned for Milestone 2; the current implementation and images use MariaDB/MySQL drivers.

#### 6.5.1 MariaDB (InnoDB)

```sql
-- Users
CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(64) PRIMARY KEY,
  username VARCHAR(255) NOT NULL UNIQUE,
  password VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Build-to-ELF mapping (stores full ELF binary)
CREATE TABLE IF NOT EXISTS build_elves (
  build_id VARCHAR(255) PRIMARY KEY,
  elf_filename VARCHAR(255) NOT NULL,
  elf_blob LONGBLOB NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Optional: for list ordering performance
CREATE INDEX IF NOT EXISTS idx_build_elves_created_at ON build_elves (created_at);

-- Upsert example (latest write wins)
-- INSERT INTO build_elves(build_id, elf_filename, elf_blob)
-- VALUES(?, ?, ?)
-- ON DUPLICATE KEY UPDATE elf_filename=VALUES(elf_filename), elf_blob=VALUES(elf_blob);
```

#### 6.5.2 PostgreSQL (planned)

```sql
-- Users
CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin','user')),
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Build-to-ELF mapping (stores full ELF binary)
CREATE TABLE IF NOT EXISTS build_elves (
  build_id TEXT PRIMARY KEY,
  elf_filename TEXT NOT NULL,
  elf_blob BYTEA NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Optional: for list ordering performance
CREATE INDEX IF NOT EXISTS idx_build_elves_created_at ON build_elves (created_at);

-- Upsert example (latest write wins)
-- INSERT INTO build_elves(build_id, elf_filename, elf_blob)
-- VALUES($1, $2, $3)
-- ON CONFLICT (build_id) DO UPDATE
--   SET elf_filename = EXCLUDED.elf_filename,
--       elf_blob     = EXCLUDED.elf_blob;
```

#### 6.6 Troubleshooting Quick Reference
- Decoder failed (500 or empty output):
  - Check backend logs and decoder output:
    ```bash
    docker compose logs -f go-backend | sed -n '1,200p'
    ```
  - Validate decoder presence and ELF path inside backend image.
  - Re-upload matching ELF or fetch by URL, then retry `/api/decode`.
- ELF not found (404 on `/api/decode`):
  - In Admin, upload `.elf` or use by-URL flow; confirm `buildId` matches.
  - Verify entry via list API:
    ```bash
    curl -s -H "Authorization: Bearer $TOKEN" http://localhost:3000/api/admin/elves | jq
    ```
- SSE progress stalls on by-URL:
  - Ensure client sends `Accept: text/event-stream`.
  - Check Nginx timeouts (Section 4.1) and backend logs.
- Unauthorized (401/403):
  - Re-login to refresh JWT; ensure `Authorization: Bearer <token>` header preserved by proxy.
  - Check server clock skew if exp/iat mismatches.
- DB storage pressure or slow queries:
  - Inspect container/volume usage and row counts; consider pruning stale ELFs.
  - Confirm indexes exist (see Section 6.5 DDL).