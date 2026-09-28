#!/usr/bin/env bash
# 本地冒烟：#19 可运行基线（登录→状态→配置→日志→优雅关闭）
set -u

REPO="/Users/junma/cc-project/gameserver"
export PATH="$HOME/.local/go/bin:$PATH"
ROOT="$(mktemp -d)"
BIN="$ROOT/gameserver"
PORT=18771
DATA="$ROOT/data"
mkdir -p "$DATA"

cd "$REPO" || exit 1
go build -o "$BIN" ./cmd/gameserver || exit 1

GAMESERVER_HOST=127.0.0.1 \
GAMESERVER_PORT="$PORT" \
GAMESERVER_DATA_ROOT="$DATA" \
GAMESERVER_ADMIN_PASSWORD=test-admin \
GAMESERVER_STATIC_DIR="$REPO/static" \
GAMESERVER_TEMPLATES_DIR="$REPO/templates" \
GAMESERVER_LAUNCH_EXECUTABLE=ProjectZomboid64 \
GAMESERVER_LAUNCH_DIRECT_EXEC=1 \
GAMESERVER_LAUNCH_EVIDENCE_REF='manifest#smoke' \
"$BIN" > "$ROOT/server.log" 2>&1 &
SRV=$!
echo "server_pid=$SRV"

ready=0
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/auth/status" 2>/dev/null; then ready=1; break; fi
  sleep 0.25
done
echo "ready=$ready"
if [ "$ready" -ne 1 ]; then
  echo '--- server log ---'; cat "$ROOT/server.log"; kill -9 "$SRV" 2>/dev/null; rm -rf "$ROOT"; exit 1
fi

JAR="$ROOT/cookies"
ORIGIN="Origin: http://127.0.0.1:$PORT"
JSON="Content-Type: application/json"

echo '--- anonymous auth status ---'
curl -sS "http://127.0.0.1:$PORT/api/auth/status"; echo
echo '--- unauthenticated protected status ---'
curl -sS -o /dev/null -w 'code=%{http_code}\n' "http://127.0.0.1:$PORT/api/status"
echo '--- missing Origin on mutating request ---'
curl -sS -b "$JAR" -H "$JSON" -X POST -d '{}' "http://127.0.0.1:$PORT/api/server/install"; echo
echo '--- login ---'
curl -sS -c "$JAR" -H "$ORIGIN" -H "$JSON" -d '{"password":"test-admin"}' "http://127.0.0.1:$PORT/api/auth/login"; echo
echo '--- status ---'
curl -sS -b "$JAR" "http://127.0.0.1:$PORT/api/status" | head -c 600; echo
echo '--- config projection ---'
curl -sS -b "$JAR" "http://127.0.0.1:$PORT/api/server/config" | head -c 500; echo
echo '--- invalid config update (range) ---'
curl -sS -b "$JAR" -H "$ORIGIN" -H "$JSON" -d '{"variables":{"MAX_PLAYERS":9999}}' "http://127.0.0.1:$PORT/api/server/config"; echo
echo '--- valid config update ---'
curl -sS -b "$JAR" -H "$ORIGIN" -H "$JSON" -d '{"variables":{"MAX_PLAYERS":24}}' "http://127.0.0.1:$PORT/api/server/config" | head -c 260; echo
echo '--- persisted state file ---'
ls -l "$DATA/servers/pz_01/state/" 2>/dev/null || echo '(no state dir)'
grep -o '"MAX_PLAYERS": *24' "$DATA/servers/pz_01/state/instance.json" 2>/dev/null || echo '(MAX_PLAYERS not persisted)'
echo '--- logs endpoint ---'
curl -sS -b "$JAR" "http://127.0.0.1:$PORT/api/server/logs?limit=5" | head -c 200; echo
echo '--- logout then replay ---'
curl -sS -b "$JAR" -c "$JAR" -H "$ORIGIN" -H "$JSON" -X POST -d '{}' "http://127.0.0.1:$PORT/api/auth/logout"; echo
curl -sS -b "$JAR" -o /dev/null -w 'replay_code=%{http_code}\n' "http://127.0.0.1:$PORT/api/status"
echo '--- SIGTERM ---'
kill -TERM "$SRV"
wait "$SRV"; code=$?
echo "server_exit=$code"
echo '--- server log tail ---'
tail -6 "$ROOT/server.log"
rm -rf "$ROOT"
