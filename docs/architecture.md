## Decode DCE Log Service — L1 Software Architecture & Design

### 0. Document Info
- System: Decode DCE Log Service
- Version: v1.3 (Restructured based on PLC L1 Template)
- Date: 2025-11-19
- Level: L1 (Architecture & High-level Design)

---

### 1. Introduction

#### 1.1. Background
The Decode DCE Log Service is designed to provide an interface for users to upload a DCE encoded log (`dce-enc.log`) and automatically decode it. Traditionally, this process might involve manual steps to identify the Build ID, locate the corresponding ELF file, and run the decoder. This project aims to automate these steps.
The backend extracts the GNU Build ID directly from the uploaded log via a short `hexdump` of the first 4 lines, resolves the corresponding ELF from the database, invokes `nvlog_decoder`, and returns a decoded log file.

#### 1.2. Purpose and Scope
**Purpose:**
Provide a focused service to decode DCE logs with minimal user input. Users upload a single `dce-enc.log`; the system auto-detects the Build ID, fetches the matching decoder ELF from an internal library, runs `nvlog_decoder`, and streams back `dce-decoded.log`.

**In Scope:**
- Frontend: Nginx static serving (SPA) + reverse proxy.
- Backend: Go REST API for business logic and decoding.
- Database: MariaDB (local) or managed DB (cloud) for storing User and ELF data.
- Containerization and health checks.
- ELF Library Management (Admin features).

**Out of Scope:**
- Complex RBAC (beyond simple Admin/User roles).
- Workflow engines or real-time pipelines.
- Cross-region active-active deployment (future).

#### 1.3. Assumptions
- Users will provide a valid `dce-enc.log` file.
- Admin users are responsible for populating the ELF library with required decoder binaries.
- The `nvlog_decoder` binary is compatible with the execution environment (Linux).
- Database availability (MariaDB or Postgres) for storing metadata and blobs.

#### 1.4. Constraints
- **Upload limits:** `client_max_body_size 512m` (Nginx) to support large ELF/log uploads.
- **Memory threshold:** Backend parses multipart uploads with a ~100MB in-memory threshold; larger files spill to disk.
- **Timeouts:** Long-running tasks (decoder, archive extraction) require extended timeouts (`proxy_read_timeout 600s`).

#### 1.5. Dependencies
- **nvlog_decoder:** Required executable for the core decoding logic.
- **Database:** MariaDB (local) or PostgreSQL (cloud) for persistence.
- **Container Runtime:** Docker / Docker Compose for deployment.
- **Nginx:** Required for serving the frontend and proxying API requests.

#### 1.6. Definitions, Acronyms, Abbreviations
- **DCE:** Display Controller Engine.
- **ELF:** Executable and Linkable Format (used for debug symbols/decoding).
- **SPA:** Single Page Application (React-based frontend).
- **JWT:** JSON Web Token (used for stateless authentication).
- **API:** Application Programming Interface.
- **NFR:** Non-functional Requirements.

---

### 2. Architectural Details

#### 2.1. High Level Description of Architecture
**Overview:**
- **Request path:** Client → Frontend (Nginx; serves SPA and proxies `/api`) → Go Backend (business logic) → Database.
- **Decode Flow (runtime):**
  1. Frontend uploads `dce-enc.log` via `POST /api/decode` (multipart; file only).
  2. Backend saves the file to a temp workspace.
  3. Backend executes `hexdump` to parse Build ID from the first few lines.
  4. Backend loads the matching ELF blob from DB by Build ID.
  5. Backend runs `nvlog_decoder` to produce `dce-decoded.log`.
  6. Backend streams the result back to the user.

**2.1.1. Service Internal Components:**
- **Frontend (React + Nginx):**
  - Vite for build. Serves static assets.
  - Reverse proxies `/api` to `go-backend:8080`.
  - Handles Health endpoint `/nginx-health`.
- **Backend (Go):**
  - Go `net/http` server.
  - Handles Auth (`/api/login`), Decode (`/api/decode`), and Admin APIs.
  - Manages `nvlog_decoder` execution and temporary file lifecycle.
  - **ELF Library Management:**
    - Admin workflow to ingest ELF artifacts via upload or URL download (background jobs).
    - Uses `readelf` to extract Build IDs.
- **Database:**
  - Stores Users (auth) and ELF artifacts (`build_elves` table).
  - Supports blob storage for ELF binaries.

**2.1.2. APIs (High-level):**
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
  - `POST /api/admin/elves/by-url` (legacy, JSON `{pushtag,url}`), and streaming/status endpoints for long-running by-URL flow:
    - `POST /api/admin/elves/by-url/start` → `{ success, jobId, created }` (creates or reuses a background job)
    - `GET /api/admin/elves/by-url/status?jobId=...` → real-time snapshot (`{ success, status, steps[], stepIndex, totalSteps, buildId?, elfName? }`)
    - `GET /api/admin/elves/by-url/stream?jobId=...` → SSE progress (`step|error|done`)
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
  | `/api/admin/elves/by-url` | POST | Bearer (admin) | Legacy non-stream flow with `{pushtag,url}`. |
  | `/api/admin/elves/by-url/start` | POST | Bearer (admin) | Start/reuse background by-URL job; returns `jobId`. |
  | `/api/admin/elves/by-url/status?jobId=<id>` | GET | Bearer (admin) | Get current snapshot of a job (`steps`, `status`, progress). |
  | `/api/admin/elves/by-url/stream?jobId=<id>` | GET | Bearer (admin) | SSE progress for the by-URL flow (`step|error|done`). |
  | `/api/admin/elves/by-url/cancel?jobId=<id>` | POST | Bearer (admin) | Cancel a running by-URL job; SSE will emit `error: cancelled by user`. |
  | `/api/admin/elves/by-url/clear?jobId=<id>` | POST | Bearer (admin) | Remove a non-running job from memory (UI “Clear”). |
  | `/healthz` | GET | None | Backend health check (200 OK). |
  | `/nginx-health` | GET | None | Frontend (Nginx) health check (200 OK; access_log off). |



#### 2.2. Tech Stack
- **Frontend:** React (Vite build) served by Nginx.
- **Backend:** Go (Golang) 1.22+ (`net/http`).
- **Database:** MariaDB (Local/Dev), Azure Database for PostgreSQL (Cloud/Planned).
- **Tools:** `hexdump`, `readelf`, `nvlog_decoder` (shipped in backend image).
- **Containerization:** Docker, Docker Compose.

#### 2.3. Deployment Strategy
- **Local/Dev:** Docker Compose (`docker-compose.yml`).
  - Runs Frontend (3000:80), Backend (internal 8080), MariaDB (3306).
  - Secrets via `.env` or environment variables.
- **Cloud/Prod:** (Planned) Azure Container Apps.
  - Frontend Container: Ingress enabled.
  - Backend Container: Internal only.
  - Managed PostgreSQL.
  - Secrets via Key Vault / ACA Secrets.

**Configuration:**
- Environment variables (`JWT_SECRET`, `MYSQL_DSN`/`DATABASE_URL`).
- Nginx config for proxy timeouts and body size limits.

#### 2.4. Summary/Response Format
**Decode Response:**
- **Success:** 200 OK.
  - Body: Binary stream of `dce-decoded.log`.
  - Headers:
    - `X-Build-Id`: The detected Build ID.
    - `X-ELF-File`: The name of the ELF file used.
    - `Content-Disposition`: attachment; filename="dce-decoded.log"
- **Failure:** 4xx/5xx JSON error.
  - Body: `{"error": "Description..."}` (may include decoder stderr output).

#### 2.5. Database Changes
- **Schema Overview:**
  - `users`: Stores user credentials and roles.
  - `build_elves`: Stores the mapping between `build_id` and the `elf_blob`.
- **Migration:**
  - Local: Auto-init scripts / one-shot migration container.
  - Cloud: Migration tool planned for Milestone 2.
- *(See Chapter 3 for detailed DDL)*.

#### 2.6. Security Consideration
- **Authentication:** JWT (HS256) for API access.
- **Network:** Backend and Database are internal-only (not exposed directly to public internet). Access is via Frontend proxy.
- **Secrets:** `JWT_SECRET` and DB credentials injected via Environment Variables; not hardcoded.
- **TLS:** Terminated at Ingress (Cloud) or HTTP-only (Local).
- **Least Privilege:** Admin role required for destructive or management operations (ELF/User mgmt).

#### 2.7. Some Load Estimates
- **Availability:** ≥ 99.9% business hours.
- **Performance:** Bound by `nvlog_decoder` CPU usage and archive extraction I/O.
- **Scalability:**
  - Backend is stateless and horizontally scalable.
  - Frontend (Nginx) is high-performance.
  - Database load is primarily on binary fetch (ELF blob) per request.

---

### 3. Operations Guide & Database Schema

This section provides command-focused guidance for developers/operators to build, run, test, and debug the system locally.

#### 3.1. Command Quick Start
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

#### 3.2. Container Management
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

#### 3.3. Health Checks
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

#### 3.4. Basic API Test Commands (Local)
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
  # Start a background job and capture jobId
  JOB_JSON=$(curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -X POST http://localhost:3000/api/admin/elves/by-url/start \
    -d '{"pushtag":"r36-abc","url":"http://.../r36-abc/latest"}')
  echo "$JOB_JSON" | jq
  JOB_ID=$(echo "$JOB_JSON" | jq -r .jobId)

  # Check status snapshot
  curl -s -H "Authorization: Bearer $TOKEN" \
    "http://localhost:3000/api/admin/elves/by-url/status?jobId=${JOB_ID}" | jq

  # Stream progress (SSE) with jobId; watch raw events:
  curl -N -H "Authorization: Bearer $TOKEN" \
    "http://localhost:3000/api/admin/elves/by-url/stream?jobId=${JOB_ID}"

  # Optional: cancel a running job
  curl -s -X POST -H "Authorization: Bearer $TOKEN" \
    "http://localhost:3000/api/admin/elves/by-url/cancel?jobId=${JOB_ID}" | jq

  # Optional: clear a finished/cancelled job
  curl -s -X POST -H "Authorization: Bearer $TOKEN" \
    "http://localhost:3000/api/admin/elves/by-url/clear?jobId=${JOB_ID}" | jq
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

#### 3.5. Database Schema (DDL)

This section provides concise DDLs for MariaDB (local) and PostgreSQL (cloud). They reflect the data model described in Section 2.5.
Note: PostgreSQL support is planned for Milestone 2; the current implementation and images use MariaDB/MySQL drivers.

**3.5.1. MariaDB (InnoDB)**

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

**3.5.2. PostgreSQL (planned)**

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

#### 3.6. Troubleshooting Quick Reference
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
  - Check Nginx timeouts and backend logs.
- Unauthorized (401/403):
  - Re-login to refresh JWT; ensure `Authorization: Bearer <token>` header preserved by proxy.
  - Check server clock skew if exp/iat mismatches.
- DB storage pressure or slow queries:
  - Inspect container/volume usage and row counts; consider pruning stale ELFs.
  - Confirm indexes exist (see Section 3.5 DDL).

---

### 4. Development Journal (Appendix)

#### 4.1. Roadmap

- **Milestone 0 — Project Bootstrap**
  - Initialize repo structure (Frontend/Backend), coding standards.
  - Add basic README and architecture docs.
  - Decide how to obtain/package `nvlog_decoder`.
  - Draft `docker-compose.yml`.

- **Milestone 1 — Local MVP (DB-backed)**
  - Backend: Auth, Admin APIs, ELF Library, Decode flow with auto-detection.
  - Frontend: Login, Log Decoder, Admin Page (Users/ELFs).
  - Database: MariaDB local.
  - Containerization: Dockerfiles, Healthchecks.

- **Milestone 2 — Cloud Baseline (ACA + Managed DB)**
  - Deploy to Azure Container Apps.
  - Managed PostgreSQL.
  - Secrets management (ACA Secrets/Key Vault).

- **Milestone 3 — Observability & Operations**
  - Centralize logs (Log Analytics).
  - Dashboards and Alerts.
  - Backup/Restore drills.

- **Milestone 4 — Optional Enhancements**
  - Async decode jobs.
  - Rate limiting.
  - Caching.
