#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

cd "$ROOT_DIR"

echo "Starting Redis in Docker..."
docker compose up -d redis

echo "Starting Go backend locally on :8000..."
(
  cd go-service
  REDIS_ADDR=127.0.0.1:6379 \
  LISTEN_ADDR=:8000 \
  BASE_TEMPLATE_PATH="$ROOT_DIR/app/latex_templates/base.tex" \
  ASSETS_TEMPLATE_DIR="$ROOT_DIR/app/latex_templates/template_images" \
  LUA_FILTER_PATH="$ROOT_DIR/ImageLuaFilter.lua" \
  ERT_WIKI_ROOT="$ROOT_DIR/ert_wiki" \
  go run ./cmd/server
) &
SERVER_PID=$!

echo "Go-only stack started"
echo "- App/API:             http://127.0.0.1:8000"
echo "- Redis (docker):      127.0.0.1:6379"
echo ""
echo "PID: server=$SERVER_PID"
echo "Press Ctrl+C to stop the local Go process."

trap 'kill "$SERVER_PID" 2>/dev/null || true' INT TERM EXIT
wait
