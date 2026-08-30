# Go Architecture (2026)

## Goals

- Provide the wiki-to-PDF API and frontend through Go.
- Keep Pandoc, Lua filter, and LuaLaTeX toolchain behavior equivalent.
- Improve operational safety: timeouts, graceful shutdown, structured logs, strict config.
- Run a single Go application service alongside Redis.

## Current API Surface

- `GET /`
- `GET /how-to-get-access-token`
- `POST /fetch`
- `POST /get-access-token`
- `POST /convert`
- `POST /generate-pdf`
- `POST /store`
- `GET /edit`
- `GET /serve-zip-project/{session_id}`

## Recommended Target Architecture

- Transport: `net/http` with `http.ServeMux` (Go 1.22+ patterns).
- Config: environment-based config loaded once on startup.
- Logging: `log/slog` JSON handler for production.
- Storage: Redis (`go-redis/v9`) for session and zip-path TTL data.
- Conversion pipeline:
  - Markdown preprocessing in Go.
  - `pandoc` command execution via `exec.CommandContext`.
  - Asset normalization and zip project generation for Overleaf.
- PDF pipeline:
  - Two-pass LuaLaTeX compile (draft then normal), bounded by context timeout.
- Static/templates: embedded SPA frontend served by the Go application.

## Best Practices (Aug 2026)

- Use request-scoped contexts with hard timeout budgets per endpoint.
- Limit request body size (`http.MaxBytesReader`) to reduce abuse risk.
- Use typed request/response DTOs and strict JSON decoding.
- Use explicit dependency injection in a service container (`App` struct).
- Treat external tools (`pandoc`, `lualatex`, `inkscape`, `plantuml`) as bounded subprocesses.
- Enforce idempotent cleanup of temp files/directories.
- Keep secrets out of logs; redact auth tokens.
- Add readiness checks that verify Redis and required binaries.
- Publish metrics/traces (OpenTelemetry) in production deployment.

## Compatibility Notes

- Keep path and payload compatibility for frontend JavaScript.
- Keep Redis key shapes and TTL windows during transition.
- Keep Lua filter invocation path configurable.
- Keep `ert_wiki` asset root configurable to support current bind mount behavior.

## Validation Checklist

- Golden tests for markdown -> latex conversion parity.
- Snapshot tests for generated PDF size/content hash tolerance.
- Contract tests for endpoint request/response formats.
- Fault injection tests for missing assets and external tool failures.
- Load tests around conversion and compile concurrency limits.
