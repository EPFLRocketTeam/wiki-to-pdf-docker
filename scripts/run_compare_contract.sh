#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

cd "$ROOT_DIR"

echo "Starting side-by-side stack (python primary + go shadow + compare proxy)..."
docker compose up -d --build web web-go redis compare-proxy

echo "Running contract replay tests..."
cd go-service
RUN_CONTRACT_TESTS=1 PYTHON_BASE_URL="http://localhost:8001" GO_BASE_URL="http://localhost:8002" go test ./tests/contract -v

cd "$ROOT_DIR"
mkdir -p artifacts
echo "Saving compare sink snapshots..."
curl -fsS "http://localhost:8000/_compare/results?limit=200" > artifacts/compare-results.json
curl -fsS "http://localhost:8000/_compare/discrepancies?limit=200" > artifacts/compare-discrepancies.json

echo "Done. Compare proxy is available on http://localhost:8000"
