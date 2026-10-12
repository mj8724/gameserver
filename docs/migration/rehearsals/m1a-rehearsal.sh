#!/usr/bin/env bash
# M1-A 隔离副本演练（不含生产/真实 PZ）
# 覆盖：启动与登录、Go 写入状态、跨进程锁栅栏、备份/篡改拒绝/恢复提升、
#      promotion 中断恢复用例、export-back → 旧格式对账、优雅关闭。
set -u

REPO="/Users/junma/cc-project/gameserver"
export PATH="$HOME/.local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct

ROOT="$(mktemp -d)"
BIN="$ROOT/gameserver"
DATA="$ROOT/data"
SERVERS="$DATA/servers"
INSTANCE="pz_01"
PORT=18801
LOG="$ROOT/server.log"
PASS=0
FAIL=0

ok()   { echo "PASS: $1"; PASS=$((PASS+1)); }
bad()  { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

cleanup() {
  [ -n "${SRV:-}" ] && kill -TERM "$SRV" 2>/dev/null
  [ -n "${LOCK_PID:-}" ] && kill "$LOCK_PID" 2>/dev/null
  sleep 0.3
  rm -rf "$ROOT"
}
trap cleanup EXIT

cd "$REPO" || exit 1
go build -o "$BIN" ./cmd/gameserver || { echo "build failed"; exit 1; }

# 1. 合成 legacy 夹具（Python 时代形状 + 未知 INI 键/注释）
mkdir -p "$SERVERS/$INSTANCE/Zomboid/Server" "$SERVERS/$INSTANCE/server_files"
cat > "$SERVERS/$INSTANCE/instance.json" <<'JSON'
{
  "instance_id": "pz_01",
  "name": "Project Zomboid Dedicated Server",
  "template_id": "project_zomboid",
  "variables": {"SERVER_NAME": "servertest", "MAX_PLAYERS": 16, "PVP_ENABLED": true},
  "ports": {"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
  "mods": {"workshop_ids": [], "mod_names": []},
  "billing": {"status": "ACTIVE", "expire_days_left": 28},
  "quota_gb": 30.0
}
JSON
cat > "$SERVERS/$INSTANCE/Zomboid/Server/servertest.ini" <<'INI'
# legacy comment must survive
Public=true
MaxPlayers=16
UnknownKey=keep-me
INI

# 1b. ADR §1.7：legacy 根典型为 0755/0644，首次 Go 启动前必须归一化权限
"$BIN" fix-permissions --data-root "$DATA" --instance "$INSTANCE" > "$ROOT/fixperm.out" 2>&1 && ok "fix-permissions ran" || bad "fix-permissions failed"

# 2. 启动真实二进制
GAMESERVER_HOST=127.0.0.1 GAMESERVER_PORT="$PORT" GAMESERVER_DATA_ROOT="$DATA" \
GAMESERVER_ADMIN_PASSWORD=rehearsal-admin GAMESERVER_STATIC_DIR="$REPO/static" \
GAMESERVER_TEMPLATES_DIR="$REPO/templates" \
GAMESERVER_LAUNCH_EXECUTABLE=ProjectZomboid64 GAMESERVER_LAUNCH_DIRECT_EXEC=1 \
GAMESERVER_LAUNCH_EVIDENCE_REF='manifest#rehearsal' \
"$BIN" > "$LOG" 2>&1 &
SRV=$!

ready=0
for _ in $(seq 1 60); do
  curl -fsS -o /dev/null "http://127.0.0.1:$PORT/api/auth/status" 2>/dev/null && { ready=1; break; }
  sleep 0.25
done
[ "$ready" -eq 1 ] && ok "server started on 127.0.0.1:$PORT" || { bad "server did not start"; cat "$LOG"; exit 1; }

JAR="$ROOT/cookies"
ORIGIN="Origin: http://127.0.0.1:$PORT"
JSON="Content-Type: application/json"
curl -sS -c "$JAR" -H "$ORIGIN" -H "$JSON" -d '{"password":"rehearsal-admin"}' \
  "http://127.0.0.1:$PORT/api/auth/login" >/dev/null && ok "login" || bad "login"

# 3. Go 写入状态（legacy 合并在内）
code=$(curl -sS -b "$JAR" -H "$ORIGIN" -H "$JSON" \
  -d '{"variables":{"MAX_PLAYERS":24}}' -o "$ROOT/update.json" -w '%{http_code}' \
  "http://127.0.0.1:$PORT/api/server/config")
[ "$code" = "200" ] && ok "config update accepted (200)" || bad "config update code=$code"
grep -q '"MAX_PLAYERS": 24' "$SERVERS/$INSTANCE/state/instance.json" \
  && ok "Go state persisted at state/instance.json" || bad "state not persisted"
grep -q 'UnknownKey=keep-me' "$SERVERS/$INSTANCE/Zomboid/Server/servertest.ini" \
  && ok "INI unknown key preserved" || bad "INI unknown key lost"

# 4. 跨进程锁栅栏：另一个 OS 进程持锁 → 变更被拒
python3 - "$SERVERS/.locks/$INSTANCE.lock" <<'PY' &
import fcntl, sys, time
handle = open(sys.argv[1], "a+")
fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
time.sleep(25)
PY
LOCK_PID=$!
sleep 1
body=$(curl -sS -b "$JAR" -H "$ORIGIN" -H "$JSON" -d '{"variables":{"MAX_PLAYERS":32}}' \
  -w '\n%{http_code}' "http://127.0.0.1:$PORT/api/server/config")
code=$(echo "$body" | tail -1)
detail=$(echo "$body" | head -1)
if [ "$code" = "409" ] && echo "$detail" | grep -q 'instance owned by another process'; then
  ok "second process holding lock blocks mutation (409 + exact detail)"
else
  bad "lock fencing: code=$code body=$detail"
fi
kill "$LOCK_PID" 2>/dev/null; LOCK_PID=""

# 5. 备份 → 篡改拒绝 → 修复 → 恢复提升
DEST="$ROOT/backups"
"$BIN" backup --data-root "$DATA" --instance "$INSTANCE" --dest "$DEST" > "$ROOT/backup.out" 2>&1 \
  && ok "backup created" || bad "backup failed: $(cat "$ROOT/backup.out")"
BACKUP_DIR=$(find "$DEST/$INSTANCE" -maxdepth 1 -mindepth 1 -type d | head -1)
printf 'corrupted' >> "$BACKUP_DIR/state/instance.json"
"$BIN" restore --data-root "$DATA" --instance "$INSTANCE" --backup "$BACKUP_DIR" > "$ROOT/restore-tampered.out" 2>&1 \
  && bad "tampered backup was accepted" || ok "tampered backup rejected"
# 修复一个字节后走 restore → promote 链路
python3 - "$BACKUP_DIR" <<'PY2'
import hashlib, json, sys
from pathlib import Path
backup = Path(sys.argv[1])
target = backup / "state" / "instance.json"
data = target.read_bytes()[:-9]
target.write_bytes(data)
digest = hashlib.sha256(data).hexdigest()
record = json.loads((backup / "backup.json").read_text())
for entry in record["files"]:
    if entry["path"] == "state/instance.json":
        entry["sha256"] = digest
        entry["size"] = len(data)
blob = b""
for entry in sorted(record["files"], key=lambda e: e["path"]):
    blob += entry["path"].encode() + b"\x00" + entry["sha256"].encode() + b"\x00"
record["checksum"] = hashlib.sha256(blob).hexdigest()
(backup / "backup.json").write_text(json.dumps(record, indent=2) + "\n")
PY2
"$BIN" restore --data-root "$DATA" --instance "$INSTANCE" --backup "$BACKUP_DIR" > "$ROOT/restore.out" 2>&1 \
  && ok "restore via backup manifest + promotion" || bad "restore failed: $(cat "$ROOT/restore.out")"

# 6. export-back → 旧格式对账
"$BIN" export-back --data-root "$DATA" --instance "$INSTANCE" > "$ROOT/export.out" 2>&1 \
  && ok "export-back ran" || bad "export-back failed: $(cat "$ROOT/export.out")"
grep -q '"MAX_PLAYERS": 24' "$SERVERS/$INSTANCE/instance.json" \
  && ok "legacy instance.json reflects Go-written value" || bad "legacy export did not match"

# 7. promotion 中断恢复用例（包内故障注入矩阵）
go test ./internal/adapters/migrate/ -run 'TestRecoveryTableRows|TestStepOneFailure|TestStepTwo|TestCommittedForwardRoll|TestBackup' -count=1 > "$ROOT/migrate.out" 2>&1 \
  && ok "migration interruption/recovery matrix" || bad "migration matrix failed"

# 8. 优雅关闭
kill -TERM "$SRV"; wait "$SRV"; code=$?
[ "$code" -eq 0 ] && ok "graceful shutdown exit=0" || bad "shutdown exit=$code"
SRV=""

echo "--- summary: pass=$PASS fail=$FAIL ---"
echo '--- server log tail ---'
tail -3 "$LOG"
exit "$FAIL"

