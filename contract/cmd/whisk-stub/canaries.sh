#!/bin/sh
# Runs each canary (templates/canary/) under whisk-stub and its canary.test, one after another.
#
#   canaries.sh <postgres base url, e.g. postgres://whisk:whisk@127.0.0.1:5432>
#
# A database named whisk_<template> is dropped and created for each run. Needs Node (the
# TypeScript template and the Inngest dev server), uv (Python), Go, curl and openssl.
set -eu
BASE=${1:?postgres base url}
ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
SECRET=whsec_canary_0123456789abcdef

go build -o /tmp/whisk-stub "$ROOT/contract/cmd/whisk-stub"

run() { # run <template> <app command...>
  T=$1; shift
  DB="whisk_$T"
  cd "$ROOT/templates/canary/$T"
  mkdir -p .whisk/dev
  printf 'STRIPE_WEBHOOK_SECRET=%s\nCANARY_SECRET=canary-secret-value-42\n' "$SECRET" > .whisk/dev/secrets.env
  psql "$BASE/postgres" -q -c "drop database if exists $DB with (force)" -c "create database $DB"
  /tmp/whisk-stub --as ana@acme.example --name Ana --database-url "$BASE/$DB" -- "$@" > "/tmp/stub-$T.log" 2>&1 &
  STUB=$!
  for _ in $(seq 1 150); do curl -sf http://127.0.0.1:3000/health >/dev/null 2>&1 && break; sleep 1; done
  curl -sf http://127.0.0.1:3000/health >/dev/null || { echo "$T: app not healthy"; tail -40 "/tmp/stub-$T.log"; kill $STUB; return 1; }
  for _ in $(seq 1 60); do curl -sf http://127.0.0.1:8288/ >/dev/null 2>&1 && break; sleep 1; done
  curl -s -X PUT http://127.0.0.1:3002/.whisk/inngest >/dev/null; sleep 6
  DESC=$(curl -s http://127.0.0.1:3001/v1/stub)
  ORG=$(printf '%s' "$DESC" | sed -n 's/.*"org_id":"\([^"]*\)".*/\1/p')
  APP=$(printf '%s' "$DESC" | sed -n 's/.*"app_id":"\([^"]*\)".*/\1/p')
  TOKEN=$(printf '%s' "$DESC" | sed -n 's/.*"service_token":"\([^"]*\)".*/\1/p')
  echo "== $T (org $ORG, app $APP)"
  RC=0
  CANARY_SERVICE_TOKEN=$TOKEN CANARY_STRIPE_SECRET=$SECRET sh canary.test http://127.0.0.1:3000 http://127.0.0.1:3001 "$ORG" "$APP" || RC=$?
  kill $STUB 2>/dev/null; wait $STUB 2>/dev/null || true
  return $RC
}

( cd "$ROOT/templates/canary/typescript" && npm ci --no-audit --no-fund --silent && npm run build --silent )
run typescript npm start
( cd "$ROOT/templates/canary/python" && uv sync --frozen --quiet )
PATH="$ROOT/templates/canary/python/.venv/bin:$PATH" run python uvicorn app.main:app --host 0.0.0.0 --port 8080
( cd "$ROOT/templates/canary/go" && go build -o server . )
run go ./server
echo "all canaries passed"
