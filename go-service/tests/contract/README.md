# Contract Tests

This suite replays recorded request payloads against both backends and compares outputs.

## Purpose

- Verify API parity while Python remains primary.
- Detect response drift in JSON endpoints.
- Detect PDF generation drift with artifact heuristics.
- Validate multi-step edit flow scenarios with response-derived variables.

## Run

1. Start both services:

   - Python on `http://localhost:8001`
   - Go on `http://localhost:8002`

2. Execute tests:

   ```bash
   cd go-service
   go test ./tests/contract -v
   ```

3. Run live replay fixtures (disabled by default):

   ```bash
   RUN_CONTRACT_TESTS=1 go test ./tests/contract -v
   ```

## Environment overrides

- `PYTHON_BASE_URL` default: `http://localhost:8001`
- `GO_BASE_URL` default: `http://localhost:8002`
- `WIKI_SAMPLE_PAGE_URL` optional, required for live `/fetch` fixture
- `WIKI_GRAPHQL_URL` optional, required for live `/fetch` fixture
- `WIKI_TOKEN` optional, required for live `/fetch` fixture

## Fixture format

Each fixture in `fixtures/*.json` contains:

- Single-step format:
   - `method`
   - `path`
   - `request`
   - `compare`
- Scenario format:
   - `steps[]`
   - each step can define `saveFields` to extract response fields as variables for next steps

`compare.type` values:

- `json`: normalizes JSON and removes ignored keys.
- `pdf`: checks content type, PDF signature, and size drift thresholds.
- `zip`: checks content type, ZIP signature, and size drift thresholds.

## Capturing production-like payloads

Use compare proxy endpoints after running mirrored traffic:

- `GET /_compare/results`
- `GET /_compare/discrepancies`

Save those request bodies as new fixture drafts, then tune comparison fields.
