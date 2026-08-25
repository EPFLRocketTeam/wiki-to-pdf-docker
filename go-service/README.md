# Go Service Scaffold

This folder contains an incremental Go migration scaffold for the wiki-to-pdf backend.

## What is implemented

- API endpoints:
  - `GET /healthz`
  - `GET /readyz`
  - `POST /fetch`
  - `POST /get-access-token`
  - `POST /convert`
  - `POST /generate-pdf`
  - `POST /store`
  - `GET /serve-zip-project/{session_id}`
- Request body limits and CORS middleware.
- Structured JSON logging (`log/slog`).
- Graceful shutdown.
- Redis-backed session/zip-path storage.
- Pandoc + Lua filter conversion.
- LuaLaTeX 2-pass compilation.
- Asset collection + Overleaf zip generation.
- Embedded SPA migration routes:
  - `GET /`
  - `GET /how-to-get-access-token`
  - `GET /edit?session_id=...`
  - `GET /api/sessions/{session_id}`
  - `GET /ui/*` static assets
- Advanced UI parity features now in SPA:
  - Monaco markdown and LaTeX editors
  - Wiki token retrieval and page fetch workflow
  - PDF preview and compile output
  - Overleaf deep-link using zip session artifacts

## What is intentionally deferred

- Complete visual parity with legacy Flask pages and all edge-case client behaviors.

## Run locally

1. Ensure dependencies are installed and available on PATH:
   - `pandoc`
   - `lualatex`
   - `inkscape`
   - `plantuml`
2. Ensure Redis is running.
3. Start the service:

```bash
cd go-service
go run ./cmd/server
```

## Environment variables

- `LISTEN_ADDR` (default `:8000`)
- `REDIS_ADDR` (default `127.0.0.1:6379`)
- `REDIS_DB` (default `0`)
- `CORS_ALLOWED_ORIGINS` (CSV, default `https://rocket-team.epfl.ch`)
- `REQUEST_BODY_LIMIT_BYTES` (default `10485760`)
- `TOOL_TIMEOUT` (default `90s`)
- `PANDOC_BINARY` (default `pandoc`)
- `LUALATEX_BINARY` (default `lualatex`)
- `LUA_FILTER_PATH` (default `/app/ImageLuaFilter.lua`)
- `BASE_TEMPLATE_PATH` (default `/app/latex_templates/base.tex`)
- `ASSETS_TEMPLATE_DIR` (default `/app/latex_templates/template_images`)
- `ERT_WIKI_ROOT` (default `/app/ert_wiki`)
- `LOG_LEVEL` (default `info`)

## Suggested rollout

1. Deploy this service in parallel with Flask.
2. Mirror a subset of traffic to Go and compare outputs.
3. Switch conversion and PDF generation endpoints first.
4. Migrate template rendering in a second step.

## Compare sink

When using the side-by-side compare proxy service, inspect automatic shadow comparison results:

- `GET /_compare/results?limit=100`
- `GET /_compare/discrepancies?limit=100`

Each record includes request id, method/path, Python status/latency, Go status/latency, and discrepancy reason.

## Contract replay tests

From this folder:

```bash
RUN_CONTRACT_TESTS=1 go test ./tests/contract -v
```

Defaults compare these base URLs:

- Python: `http://localhost:8001`
- Go: `http://localhost:8002`

Override with `PYTHON_BASE_URL` and `GO_BASE_URL`.
