#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

cd "$ROOT_DIR"

echo "Starting Go-only stack (web-go + redis)..."
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d --build web-go redis

echo "Waiting for Go service health endpoint..."
for i in $(seq 1 30); do
	if curl -fsS "http://localhost:8000/healthz" >/dev/null; then
		break
	fi
	sleep 1
done

echo "Running Go unit tests..."
cd go-service
go test ./...

cd "$ROOT_DIR"
mkdir -p artifacts
echo "Running API smoke requests..."
curl -fsS -X POST "http://localhost:8000/convert" \
	-H "Content-Type: application/json" \
	-d '{"markdown":"# Smoke\n\nGo-only stack.","template":"space-race","author":"Smoke","date":"2026-08-25","title":"Go-only","documentId":"2026_P_SS_DOC","footerText":"","lineNumbersEnabled":false}' \
	> artifacts/go-convert-response.json

curl -fsS -X POST "http://localhost:8000/generate-pdf" \
	-H "Content-Type: application/json" \
	-d '{"title":"Go-only","latex_code":"\\documentclass{article}\n\\begin{document}\nGo-only stack smoke\\end{document}"}' \
	> artifacts/go-smoke.pdf

echo "Done. Go-only service is available on http://localhost:8000"
