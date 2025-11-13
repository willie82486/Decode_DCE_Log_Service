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
  - Two-tier deployment: frontend (Nginx SPA + reverse proxy), backend (Go API), database (MariaDB for local; managed DB for cloud).
  - Operational simplicity: health checks, container log rotation, baseline security/observability.
  - Scalability: backend horizontally scalable.

---

### 2. Scope & Out of Scope
- In Scope: frontend (Nginx static + proxy), backend (Go REST API), database (MariaDB locally, cloud managed DB), containerization and health checks.
- Out of Scope: complex RBAC, workflow engines, real-time pipelines, cross-region active-active (future).

---

### 3. High-level Architecture
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

---

### 4. Component Design
- Frontend (React + Nginx)
  - Vite for build. Nginx serves `index.html` and static assets from `/usr/share/nginx/html`.
  - Reverse proxy `/api` to `go-backend:8080` (same Docker network).
  - Health endpoint `/nginx-health` returns 200 with `access_log off`.
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

---

### 5. APIs (High-level)
- Auth
  - `POST /api/login`: body `{username,password}`; returns `{success, role, token}` (HS256; `JWT_SECRET`).
- Decode
  - `POST /api/decode` (Bearer token required): multipart upload with field `file` only.
    - Behavior: save → extract Build ID from hexdump → fetch ELF by Build ID → decode → return file.
    - Headers: `X-Build-Id`, `X-ELF-File` on success; on failure, returns clear error messages and logs decoder output.
- Admin
  - `GET /api/admin/users`, `POST /api/admin/users`, `DELETE /api/admin/users?id=<id>`
  - `GET /api/admin/elves` → `[ { buildId, elfFileName }, ... ]`
  - `DELETE /api/admin/elves?buildId=<id>`
  - `POST /api/admin/elves/upload` (multipart `elf`)
  - `POST /api/admin/elves/by-url` (JSON `{pushtag,url}`), and `GET /api/admin/elves/by-url/stream?pushtag=...&url=...` (SSE progress)
- Health
  - `GET /healthz` → 200 OK

---

### 6. Data Model
- Local/dev DB: MariaDB (via docker-compose).
- Tables:
  - `users`: `id` (PK), `username` (UNIQUE), `password` (plaintext for dev), `role`, `created_at`.
  - `build_elves`: `build_id` (PK), `elf_filename`, `elf_blob` (LONGBLOB), `created_at`.
- Connection (local):
  - `MYSQL_DSN=dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true`

---

### 7. Deployment & Environments
- Pipeline: Build images → run locally with Docker Compose; in cloud, publish to registry and deploy (e.g., Azure Container Apps).
- Frontend Container:
  - Ingress enabled (HTTP for local). Proxies `/api` to backend within the network.
- Backend Container:
  - No host port published in production-mode compose; reachable via frontend proxy.
  - Secrets: `JWT_SECRET`, `MYSQL_DSN`/`DATABASE_URL` (cloud: prefer managed DB and SSL).
- Database:
  - MariaDB (compose) with mounted init script and a one-shot migration container.
- Scaling:
  - Frontend: scale by requests/connections; min 1 instance.
  - Backend: scale by CPU/HTTP concurrency; min 1 instance.

---

### 8. Networking & Security
- Perimeter: Frontend exposed; backend internal-only; DB internal.
- TLS: Terminate at ingress in cloud (fronting proxy/ingress). For local, HTTP-only.
- Secrets: Use environment variables; in cloud, use ACA Secrets/Key Vault.
- JWT: HS256 with `JWT_SECRET`. Authorization header is preserved by Nginx proxy.
- Least privilege: DB user limited; container logs rotated.

---

### 9. Health & Logging
- Health Checks:
  - Frontend: `/nginx-health` (200).
  - Backend: `/healthz` (200).
- Logs:
  - Docker `json-file` with rotation.
  - Backend prints detailed decoder stdout/stderr on failure; includes `X-Build-Id` in responses to aid troubleshooting.

---

### 10. Non-functional Requirements (NFRs)
- Availability: ≥ 99.9% business hours.
- Performance: bound by decoder and archive operations; backend horizontally scalable.
- Security: JWT, secrets management, minimal exposure, private networking in cloud.
- Operability: health probes, log rotation, structured error messages, and diagnostics.

---

### 11. Risks & Mitigations
- External artifacts unavailable or slow:
  - SSE progress for by-URL fetch; explicit error propagation.
- `nvlog_decoder` compatibility/resource usage:
  - Capture outputs; validate produced file; use worker model if needed later.
- Large files/long-running tasks:
  - Future option: async jobs and status tracking UI.

---

### 13. Container Images
- Backend
  - Build stage: `golang:1.22-alpine` (compile static binary).
  - Runtime stage: `debian:bookworm-slim` with `binutils` (for `readelf`) and `bsdextrautils` (for `hexdump`), plus `curl`, `bzip2`, `tar`.
  - Ships `nvlog_decoder` at `/usr/local/bin/nvlog_decoder`.
- Frontend
  - `nginx:alpine`. Serves built SPA and proxies `/api` to backend.

---

### 14. Current UI Behavior
- Login (role-aware) → Log Decoder page (upload `dce-enc.log`) → Download decoded log.
- Admin page:
  - Manage users.
  - Manage ELF library (details below).

---

### 15. ELF Library Management (Detailed)

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
### 16. Operations Guide (Local)

This section provides command-focused guidance for developers/operators to build, run, test, and debug the system locally.

#### 16.1 Command Quick Start
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

#### 16.2 Container Management
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
  docker compose restart dce-log-go-backend
  ```
- Stop all and remove (keep volumes):
  ```bash
  docker compose down
  ```
- Stop all and remove including volumes (DANGER: wipes DB data):
  ```bash
  docker compose down -v
  ```

#### 16.3 Health Checks
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

#### 16.4 Basic API Test Commands (Local)
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