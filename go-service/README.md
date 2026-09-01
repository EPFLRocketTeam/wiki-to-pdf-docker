# Go Service

This folder contains the Go implementation of the wiki-to-pdf application.

## What is implemented

- API endpoints:
  - `GET /healthz`
  - `GET /readyz`
  - `POST /fetch`
  - `POST /get-access-token`
  - `POST /convert`
  - `POST /generate-pdf`
  - `POST /store`
  - `POST /editor-sessions`
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

## Run locally

1. Ensure dependencies are installed and available on PATH:
  - `pandoc`
  - `lualatex`
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
- `REQUEST_BODY_LIMIT_BYTES` (default `104857600`, 100 MiB)
- `HTTP_READ_TIMEOUT` (default `2m`)
- `HTTP_WRITE_TIMEOUT` (default `6m`)
- `TOOL_TIMEOUT` (default `90s`)
- `PANDOC_BINARY` (default `pandoc`)
- `LUALATEX_BINARY` (default `lualatex`)
- `LUA_FILTER_PATH` (default `/app/ImageLuaFilter.lua`)
- `BASE_TEMPLATE_PATH` (default `/app/latex_templates/base.tex`)
- `ASSETS_TEMPLATE_DIR` (default `/app/latex_templates/template_images`)
- `ERT_WIKI_ROOT` (default `/app/ert_wiki`)
- `LOG_LEVEL` (default `info`)

## Open a prefilled editor session

`POST /editor-sessions` accepts markdown and the conversion settings, stores
them for 24 hours, and returns an `edit_url`. Open that URL in a browser tab to
edit and compile the supplied document. The response contains no markdown, so
large documents and their contents do not need to be placed in a URL.

```json
{
  "markdown": "# Launch report",
  "title": "Launch report",
  "author": "Jane Doe",
  "date": "2026-08-30",
  "documentId": "RPT-42",
  "template": "space-race",
  "footerText": "Internal",
  "lineNumbersEnabled": false,
  "images": [
    {
      "path": "/general/rocket.png",
      "content": "iVBORw0KGgoAAAANSUhEUg..."
    }
  ]
}
```

For conversion, send each Wiki.js image in the `images` array with the exact
path used by the Markdown image (for example, `/general/rocket.png`). `content`
is a base64-encoded string. The service retains editor-session images privately
and uses them on the later `POST /convert`; standalone conversion requests can
provide the same `images` array directly. The service packages matching images
without Wiki.js credentials or authenticated image requests.

`imageBaseUrl` and `imageAuthToken` remain supported for existing clients, but
new integrations should supply `images` directly.

## Contract replay tests

From this folder:

```bash
RUN_CONTRACT_TESTS=1 go test ./tests/contract -v
```

Defaults target the Go backend at `http://localhost:8000`.

Override with `GO_BASE_URL` if needed.
