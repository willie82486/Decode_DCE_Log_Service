## Decode DCE Log Service — L1 Software Architecture & Design 

### 0. Document Info
- System: Decode DCE Log Service
- Version: v1.0 (Draft)
- Date: 2025-11-09
- Level: L1 (Architecture & High-level Design)

---

### 1. Introduction & Goals
- Purpose: Provide an interface to upload `dce-enc.log` and input `pushtag/buildId`. The backend downloads and extracts required artifacts, locates the target ELF, invokes `nvlog_decoder`, and returns a decoded log file.
- Goals:
  - Two-tier cloud deployment: frontend and backend as containers on Azure Container Apps (ACA); database on Azure managed DB (PostgreSQL recommended).
  - Reduce operations burden: PaaS, health checks, container log rotation, baseline security and monitoring.
  - Scalability: enable horizontal scaling for API instances.

---

### 2. Scope & Out of Scope
- In Scope: frontend (Nginx static serving + reverse proxy), backend (Go REST API), database (Azure managed), deployment (ACA/ACR), health checks and logs.
- Out of Scope: complex RBAC, workflow engines, real-time streaming pipelines, cross-region active-active (planned later).

---

### 3. High-level Architecture
- Request path: Client → ACA Ingress (Frontend, TLS terminates at ACA) → Nginx (serves SPA and proxies `/api`) → Go Backend (business logic) → Azure Database (managed DB).
- Two Container Apps:
  - Container App: frontend (Ingress enabled; Nginx serves static SPA and proxies `/api` to backend).
  - Container App: backend (Ingress disabled; internal-only).
  - Database: Azure Database for PostgreSQL (Flexible Server preferred; Private Endpoint or restricted public access).
- Flow:
  - Frontend: Nginx returns static files; routes `/api/*` to backend.
  - Backend: creates temp work dir → downloads/extracts archives → locates ELF → calls `nvlog_decoder` → returns decoded file.

---

### 4. Component Design
- Frontend (React + Nginx)
  - Vite is used only for build (no dev server in production). Nginx serves `index.html` and static assets from `/usr/share/nginx/html`.
  - Reverse proxy `/api` to `go-backend:8080` over the container network.
  - Health endpoint `/nginx-health` returns 200 and has `access_log off`.
- Backend (Go)
  - Endpoints: `POST /api/decode`, `POST /api/login`, `GET/POST /api/admin/users`, `GET /healthz`.
  - Decode flow: multipart upload → temp dir → download/extract → find `display-t234-dce-log.elf` → call `nvlog_decoder` → return `dce-decoded.log`.
  - Health: `/healthz` returns 200 OK.
- Container Images
  - Backend: build on `golang:alpine`, runtime on `alpine` with `bzip2 tar curl`; ships `nvlog_decoder`.
  - Frontend: `nginx:alpine`, install `curl` for healthcheck probes.

---

### 5. APIs (High-level)
- `POST /api/decode`: multipart upload `file` with fields `pushtag` and `buildId`; performs decode via pushtag→url mapping in DB; returns `dce-decoded.log` (response includes `X-ELF-File` header).
- `POST /api/login`: login against DB (local dev uses plaintext; production will use hashed passwords).
- `GET /api/admin/users`: list users (admins first); password not included in response.
- `POST /api/admin/users`: create user with `{username,password,role}` where role ∈ {admin,user}.
- `DELETE /api/admin/users?id=<id>`: delete user by id.
- `GET /api/admin/pushtags`: list existing pushtags (names only).
- `POST /api/admin/pushtags`: upsert `{pushtag,url}` mapping.
- `GET /healthz`: liveness/readiness healthcheck.

---

### 6. Data Model (Planned DB)
- DB: Azure Database for PostgreSQL (Flexible Server).
- Proposed tables:
  - `users`: id (string PK), username (UNIQUE), password (plaintext for local dev), role, created_at.
  - `pushtag_urls`: pushtag (PK), url, created_at.
- Connection:
  - Inject `DATABASE_URL` via ACA Secrets; enforce SSL (e.g., `sslmode=require`).
  - Prefer Private Endpoint; otherwise restrict public firewall to ACA egress and enforce SSL.

---

### 7. Deployment & Environments (Azure)
- Pipeline:
  - Build images → push to Azure Container Registry (ACR) → create ACA Environment (optionally VNet integrated) → deploy frontend/backend Container Apps.
- Frontend Container App:
  - Ingress: enabled (HTTPS), TLS managed by ACA or a fronting Front Door.
  - Env/Secrets: only if needed (usually same-origin `/api` needs none).
- Backend Container App:
  - Ingress: disabled (internal only).
  - Secrets: `DATABASE_URL` and any other sensitive settings.
- Database:
  - Azure Database for PostgreSQL with automated backups and HA; Private Endpoint preferred.
- Scaling:
  - Frontend: autoscale by HTTP requests/connections; min 1 instance.
  - Backend: autoscale by CPU or HTTP concurrency; min 1 instance.

---

### 8. Networking & Security
- Perimeter:
  - External entry via ACA Ingress (or Azure Front Door/Application Gateway in front). Nginx and backend do not need public exposure.
- TLS:
  - Terminate TLS at ACA/Front Door. Internal hop to frontend can be HTTP (simple) or end-to-end HTTPS (stricter, requires Nginx certs).
- Secrets:
  - Use ACA Secrets/Azure Key Vault. Do not bake secrets into images.
- Least Privilege:
  - Minimal ACR Pull role, least-privileged DB user, NSG rules restricted to necessary sources/ports.

---

### 9. Health & Logging
- Health Checks:
  - Frontend: `/nginx-health` (200; `access_log off`) for liveness/readiness.
  - Backend: `/healthz` (200).
  - Used by ACA probes for startup/liveness/readiness and auto-restarts.
- Logs:
  - Container-level log rotation with Docker `json-file` driver (`max-size`, `max-file`) to avoid disk exhaustion (for local/VM).
  - In Azure: send stdout/stderr to Log Analytics; optionally integrate Application Insights for request tracing and dependency timing.

---

### 10. Non-functional Requirements (NFRs)
- Availability: ≥ 99.9% during business hours.
- Performance: end-to-end latency driven by external downloads/extractions and decoder runtime; backend horizontally scalable.
- Security: TLS, secrets management, least privilege, private networking, optional WAF.
- Operability: health probes, log rotation, monitoring/alerts, restart/redeploy procedures.

---

### 11. Risks & Mitigations
- External source instability (download/extract failures):
  - Retries, clear error reporting, mirror sources if possible.
- `nvlog_decoder` compatibility/resource usage:
  - Monitor CPU/memory; consider async jobs and concurrency caps.
- Large files/long-running tasks:
  - Introduce a job queue and worker model; track job states and avoid blocking sync APIs.

---

### 12. Operations Guide (Local & VM Environments)

This section provides practical references for deploying, verifying, and operating the system in local development or a VM environment.

#### Deployment Overview
- **Production Docker Compose (`HTTP-only`):**
  - The **frontend** service exposes port `80:80`, acting as a reverse proxy for all `/api` traffic to the backend.
  - The **backend** service runs internally and does *not* publish any ports to the host.
- **Database:** 
  - Uses a MariaDB container named `mariadb`, preconfigured with the database `dce_logs`.
  - The backend connects using:  
    `MYSQL_DSN=dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true`

#### Health Checks
- **Frontend Health:**
  - General: `curl -I http://<host>/nginx-health`
  - Local example: `curl -sS http://localhost:3000/nginx-health`
- **Backend Health:**
  - Accessible from inside a container or via Docker network:  
    `curl -I http://localhost:8080/healthz`
- **Database Health:**
  - Inspect health:  
    `docker inspect dce-log-mariadb --format '{{json .State.Health}}' | jq`
  - If you see errors like `mysqladmin: not found`, use:
    - `docker exec dce-log-mariadb which mariadb-admin`
    - `docker exec dce-log-mariadb which mysqladmin`
  - If necessary, update `docker-compose.yml` healthcheck accordingly:  
    `test: ["CMD-SHELL", "mariadb-admin ping -h 127.0.0.1 --silent || exit 1"]`

#### Container Management
- **View container status (including health):**  
  `docker ps`
- **View logs (last 200 lines):**
  - MariaDB: `docker logs dce-log-mariadb --tail 200`
  - Backend: `docker logs dce-log-backend --tail 200`
- **Live log streaming:**  
  (add `-f`) e.g., `docker logs -f dce-log-backend`
- **Log rotation:**  
  Enabled using the Docker `json-file` driver with limits on log size and rotations to prevent disk exhaustion.

#### Basic API Test Commands (Local)
- **Create user:**  
  `curl -s -X POST http://localhost:3000/api/admin/users -H 'Content-Type: application/json' -d '{"username":"admin1","password":"password123","role":"admin"}'`
- **List users:**  
  `curl -s http://localhost:3000/api/admin/users`
- **Delete user:**  
  `curl -s -X DELETE 'http://localhost:3000/api/admin/users?id=<USER_ID>'`
- **Upsert pushtag mapping:**  
  `curl -s -X POST http://localhost:3000/api/admin/pushtags -H 'Content-Type: application/json' -d '{"pushtag":"r36-abc","url":"http://buildbrain/.../r36-abc/latest"}'`
- **List pushtags:**  
  `curl -s http://localhost:3000/api/admin/pushtags`
- **Decode upload:**  
  `curl -s -X POST http://localhost:3000/api/decode -F pushtag=r36-abc -F buildId=1234 -F file=@./dce-enc.log -o dce-decoded.log`

#### Command Quick Reference
- **Build and start services:**  
  `docker compose up -d --build`
- **Force backend rebuild (resolve Go dep/cache issues):**  
  `docker compose build --no-cache go-backend && docker compose up -d`
- **Database Volumes & Data:**
  - List volumes: `docker volume ls`
  - Inspect a volume: `docker volume inspect dce_db`
  - Remove all containers & volumes (WARNING: deletes data!):  
    `docker compose down -v`

#### Database Initialization & Migrations
- **Initialization scripts (first run only):**
  - The schema file `db/init/001_schema.sql` is mounted to the MariaDB container at `/docker-entrypoint-initdb.d/001_schema.sql` via:
    - `mariadb.volumes: - ./db/init:/docker-entrypoint-initdb.d:ro`
  - MariaDB runs scripts in `/docker-entrypoint-initdb.d` only when the data directory is empty (i.e., first run with a fresh volume).
- **Versioned migrations (on every deploy):**
  - A one-shot migration service runs before the backend:
    - Service: `migrate` (image `migrate/migrate:4`)
    - Command: `migrate -path /migrations -database "mysql://dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?multiStatements=true" up`
    - Mounts: `./db/migrations:/migrations:ro`
    - Compose dependency: backend waits on `migrate` with `condition: service_completed_successfully`
  - Migration files live in `db/migrations/`:
    - `0001_create_core_tables.up.sql` / `0001_create_core_tables.down.sql`
  - For local/dev convenience, initial `up.sql` uses `IF NOT EXISTS` to be idempotent alongside the init script.
- **If the volume already exists and tables are missing (no data loss):**
  - Apply the schema manually:
    ```
    docker exec -i dce-log-mariadb sh -c 'mariadb -udce_user -pdce_pass < /docker-entrypoint-initdb.d/001_schema.sql'
    ```
- **Reset DB (data loss) to re-trigger init scripts:**
  - Useful for local dev if you want a clean slate:
    ```
    docker compose down -v
    docker compose up -d --build
    ```
- **Healthcheck tooling:**
  - The container uses `mariadb-admin` for health checks (not `mysqladmin`):
    - `test: ["CMD-SHELL", "mariadb-admin ping -h 127.0.0.1 --silent || exit 1"]`
  - Validate tool presence:
    ```
    docker exec dce-log-mariadb which mariadb-admin
    ```

---

### 13. Roadmap (from zero)
- Milestone 0 — Project Bootstrap
  - Initialize repo structure (Frontend/Backend), coding standards.
  - Add basic README and architecture docs (this file).
  - Confirm how to obtain `nvlog_decoder`.
- Milestone 1 — Local MVP (DB-backed)
  - Backend:
    - Implement `POST /api/login` against local MariaDB (plaintext for dev; plan to hash in prod).
    - Implement `/api/admin/users` (GET/POST/DELETE) backed by DB; enforce unique username; admin-first ordering on list; do not expose password in responses.
    - Implement `/api/admin/pushtags` (GET/POST) backed by DB.
    - Implement `/api/decode` to resolve `pushtag → url` from DB before download/decode; return `X-ELF-File`.
    - Provide `/healthz` health endpoint.
  - Frontend:
    - LoginPage (calls `/api/login` and captures role).
    - LogDecoder (multipart upload with `pushtag` and `buildId`).
    - AdminPage (add/list/delete users; add/list pushtags).
    - Nginx serves SPA and reverse proxies `/api`; `/nginx-health` for probes.
  - Database (MariaDB):
    - Compose service `mariadb:11` (local-only). Environment variables:
      - `MARIADB_DATABASE=dce_logs`, `MARIADB_USER=dce_user`, `MARIADB_PASSWORD=dce_pass`, `MARIADB_ROOT_PASSWORD=rootpassword`
    - Backend connection string injected via env:
      - `MYSQL_DSN=dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true`
    - Auto-create tables on backend startup:
      - `users(id, username UNIQUE, password, role, created_at)`
      - `pushtag_urls(pushtag PK, url, created_at)`
    - Healthcheck: `mysqladmin ping` (compose healthcheck)
    - Note: For local development, passwords are stored in plaintext; in cloud, switch to hashing (e.g., bcrypt).
    - Optional: add a named volume to persist DB data across container rebuilds.
  - Containerization:
    - Dockerfiles for frontend/backend; `docker-compose.yml` including MariaDB; healthchecks and container log rotation.
- Milestone 2 — Cloud Baseline (ACA + Managed DB)
  - Build/push images to ACR; deploy two Container Apps (frontend with Ingress, backend internal-only).
  - Configure secrets via ACA (e.g., `DATABASE_URL`); bind custom domain and TLS at ACA/Front Door.
  - Provision Azure managed DB (PostgreSQL preferred); wire up `DATABASE_URL` (SSL required).
  - Migrate `login/admin users` to managed DB; add basic audit logs.
- Milestone 3 — Observability & Operations
  - Send logs to Log Analytics; basic dashboards and alerts.
  - Backup/restore drill for DB; initial load/perf tests; cost baseline and right-sizing.
- Milestone 4 — Optional Enhancements (do when time allows)
  - Async decode jobs (Queue + Worker) [OPTIONAL].
  - Frontend job status UI [OPTIONAL].
- Out of Scope for Now
  - “Hard” security hardening (e.g., WAF tuning, end-to-end TLS everywhere, threat modeling workshops) — intentionally deferred. Keep baseline security only: TLS at ingress, least privilege, secrets management, private networking preference.

---