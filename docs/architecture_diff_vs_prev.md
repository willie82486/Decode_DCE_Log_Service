## Differences vs Previous Architecture Document

Reference (previous version): `https://github.com/willie82486/Decode_DCE_Log_Service/blob/0f9ad833fd7423a80876ef197fdfc1d11743035a/docs/architecture.md`

- Decode input
  - Previous: User must provide `pushtag` and `buildId` with the uploaded `dce-enc.log`.
  - Now: User uploads `dce-enc.log` only. Backend extracts Build ID from the log via a short `hexdump` of the first 4 lines (bytes `0x20..0x33` → 40-hex).
- Build ID resolution
  - Previous: Provided by user.
  - Now: Auto-derived in backend; response header `X-Build-Id` added for traceability.
- Decoder invocation
  - Previous: Not specified or implied older parameters; path composition might include `__<pushtag>__<buildId>`.
  - Now: `nvlog_decoder -d none -i <log> -o <out> -e <elfPath> -f DCE` with strict checks; `-e` receives the actual temp ELF path (no augmented suffix).
- Error handling & observability
  - Previous: Generic error messages; potential “404 page not found” when decoder produced no output.
  - Now: Decoder `CombinedOutput` captured; explicit check for output file existence; clearer 404 (missing ELF) vs 500 (DB/decoder failure); `X-ELF-File` and `X-Build-Id` headers on success.
- Frontend UI
  - Previous: Log Decoder page required `pushtag` and `buildId` fields.
  - Now: Only file upload is required.
- Admin features
  - Previous: Mentioned pushtag mapping endpoints and a simpler description of ELF retrieval.
  - Now (ELF Library Management expanded & revised):
    - Data model changed to `build_elves (build_id PK, elf_filename, elf_blob, created_at)` with upsert semantics.
    - Upload flow (`POST /api/admin/elves/upload`): extracts Build ID from ELF; preserves filename if matches `display-t234-dce-log.elf__<pushtag>__<40hex>`, else normalizes to `display-t234-dce-log.elf__<buildId>`; stores full blob.
    - Fetch-by-URL flow (upgraded to background jobs):
      - Previous: Triggered directly via `/elves/by-url` and observed via `/elves/by-url/stream?pushtag=...&url=...`; prone to interruption after page refresh; no cancellation.
      - Now: Runs as a resumable background job that supports resume-after-refresh, cancellation, and clearing.
        - `POST /api/admin/elves/by-url/start` → create/reuse a job, returns `jobId`
        - `GET /api/admin/elves/by-url/status?jobId=...` → current snapshot (steps, status, progress)
        - `GET /api/admin/elves/by-url/stream?jobId=...` → SSE progress (`step|error|done`; includes catch-up)
        - `POST /api/admin/elves/by-url/cancel?jobId=...` → cancel a running job (SSE will receive `error: cancelled by user`)
        - `POST /api/admin/elves/by-url/clear?jobId=...` → clear a finished/cancelled job
      - Work performed remains the same: download `full_linux_for_tegra.tbz2`, extract overlay, locate `display-t234-dce-log.elf`, read Build ID, store into DB; SSE still reports `step|error|done` with final `{buildId, elfFileName}`.
    - Listing & deletion: `GET /api/admin/elves` returns `{ buildId, elfFileName }` (newest first); `DELETE /api/admin/elves?buildId=...` removes entry.
    - Admin UI: provides Upload and Fetch-by-URL (SSE progress + `localStorage` persistence + resumability), list and delete; while running, “Fetch & Store” is disabled and a “Stop” button is shown to cancel; “Clear” is enabled only after completion or cancellation; a progress bar is added. Also fixes duplicated progress after refresh (use SSE catch-up and have the client skip already-known steps).
- Data model
  - Previous (planned): `users`, `pushtag_urls`.
  - Now (implemented): `users`, `build_elves (build_id PK, elf_filename, elf_blob, created_at)`.
- Container runtime
  - Previous: Backend runtime on `alpine` with `bzip2 tar curl`.
  - Now: Backend runtime on `debian:bookworm-slim` with `binutils` (for `readelf`) and `bsdextrautils` (for `hexdump`) to support build-id extraction from logs and ELF; `nvlog_decoder` shipped at `/usr/local/bin/nvlog_decoder`.
- API section
  - Previous: `POST /api/decode` required `buildId` and optionally `pushtag`.
  - Now: `POST /api/decode` requires only `file`; backend auto-extracts Build ID and returns `dce-decoded.log`.
- Async job lifecycle & TTL (new)
  - New: Backend adds a JobManager with TTL-based auto-reaping:
    - `BYURL_JOB_FINISHED_TTL` (default 30m): retention window for done/error jobs
    - `BYURL_JOB_RUNNING_TTL` (default 12h): stale timeout for running jobs with no updates
    - `BYURL_JOB_REAPER_INTERVAL` (default 1m): sweep frequency
  - Compose files include recommended defaults under the backend service `environment`.
- Documentation structure
  - Updated wording across sections (Introduction, Components, APIs, Data Model, Containers) to match the auto-detection flow and current code behavior.


