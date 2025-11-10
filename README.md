# Decode DCE Log Service

A containerized web service that decodes DCE logs by downloading and extracting required artifacts and invoking `nvlog_decoder`, served via Nginx (frontend) and a Go backend API.

## Documentation
- English L1 Architecture: [`docs/architecture.md`](docs/architecture.md)

## Quick Start (Local/VM)
```bash
docker compose -f docker-compose.prod.yml up -d --build
curl -I http://localhost/nginx-health
```


