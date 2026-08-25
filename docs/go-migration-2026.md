# Go Migration Plan (2026)

## Goals

- Preserve existing API behavior while replacing the Flask runtime with Go.
- Keep Pandoc, Lua filter, and LuaLaTeX toolchain behavior equivalent.
- Improve operational safety: timeouts, graceful shutdown, structured logs, strict config.
- Support a zero-downtime migration by running Python and Go services in parallel.

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
- Static/templates:
  - Phase 1: API-first migration, keep existing frontend served by Python or reverse proxy.
  - Phase 2: move templates to Go (`html/template`) or an SPA frontend.

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

## Migration Strategy

1. Phase A: Stand up Go service with API compatibility for core endpoints.
2. Phase B: Put Go behind reverse proxy on a shadow path and compare outputs.
3. Phase C: Route write and conversion traffic to Go; keep Python as fallback.
4. Phase D: Migrate template rendering or serve a decoupled frontend.
5. Phase E: Remove Python stack and simplify image/container layers.

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
