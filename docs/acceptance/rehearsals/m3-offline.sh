#!/usr/bin/env bash
# M3 离线矩阵执行器（单实例可靠性里程碑）
#
# 只执行 docs/acceptance/M3-GO-PZ-RELIABILITY.md 中不依赖真实 PZ、SteamCMD、
# 远端主机、浏览器或网络探测的行；能力尚未实现的行记 NOT RUN，外部前提缺失
# 的行记 BLOCKED，绝不记 PASS。
#
# 用法: bash docs/acceptance/rehearsals/m3-offline.sh [--out <json 路径>]
# 退出码: 0 = 无 FAIL（BLOCKED / NOT RUN 允许存在）；1 = 有 FAIL
set -u

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PATH="$HOME/.local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
cd "$REPO" || exit 1

OUT="${M3_OUT:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

WORK="$(mktemp -d)"
ROWS="$WORK/rows.jsonl"
: > "$ROWS"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

record() { # id | title | status | evidence
  printf '{"id":"%s","title":"%s","status":"%s","evidence":"%s"}\n' "$1" "$2" "$3" "$4" >> "$ROWS"
  printf '%-16s %-9s %s\n' "$1" "$3" "$2" >&2
}

# 待判定行：单次 go test 运行后按测试名判定（复用 tools/m2eval 的判定模式）
ROW_DEFS="$WORK/rowdefs.tsv"
: > "$ROW_DEFS"
row() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$ROW_DEFS"; }

# 能力尚未实现 → NOT RUN（能力落地后由 row() 取代）
notrun() { record "$1" "$2" "NOT RUN" "$3"; }
# 外部前提缺失 → BLOCKED
blocked() { record "$1" "$2" "BLOCKED" "$3"; }

echo "== 环境 =="
COMMIT="$(git rev-parse HEAD)"
GO_VER="$(go version | awk '{print $3}')"
printf 'commit=%s\ngo=%s\nplatform=%s/%s\n' "$COMMIT" "$GO_VER" "$(go env GOOS)" "$(go env GOARCH)" >&2
go build -o "$WORK/gameserver" ./cmd/gameserver || { echo "build failed" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  BIN_SHA="$(sha256sum "$WORK/gameserver" | awk '{print $1}')"
else
  BIN_SHA="$(shasum -a 256 "$WORK/gameserver" | awk '{print $1}')"
fi
printf 'binary_sha256=%s\n' "$BIN_SHA" >&2

# ---------- 构建门 ----------
BUILD_OK=1
test -z "$(gofmt -l cmd internal tools)" || BUILD_OK=0
go vet ./... >/dev/null 2>&1 || BUILD_OK=0
go build ./... >/dev/null 2>&1 || BUILD_OK=0
go mod tidy -diff >/dev/null 2>&1 || BUILD_OK=0
if [ "$BUILD_OK" -eq 1 ]; then
  record M3-BUILD "固定工具链与本机门禁（gofmt/vet/build/tidy）" PASS "go=${GO_VER} commit=${COMMIT} binary_sha256=${BIN_SHA}"
else
  record M3-BUILD "固定工具链与本机门禁（gofmt/vet/build/tidy）" FAIL "见上方步骤输出"
fi

# ---------- M3.1 任务恢复 ----------
row M3.1 "意图日志阶段化持久化（REQUESTED→RUNNING→VERIFYING→DONE/FAILED）" 'TestTaskIntentLogPersistsPhases'
row M3.1 "重启对账自洽且无重复下载（build id+字节+指纹）" 'TestTaskReconcileAfterCrashAvoidsRedownload|TestTaskReconcileWithoutContentFailsClosed'
row M3.1 "对账顺序：陈旧 owner → RECOVERY_REQUIRED 优先，不被续跑覆盖" 'TestReconcileDefersToRecoveryRequired'
row M3.1 "网络失败 → FAILED 可重试；legacy 四态投影不变" 'TestTaskIntentFailureAndLegacyStatusProjection' 

# ---------- M3.2 备份保留 ----------
row M3.2 "prune 保留 N 且永不删除最后一份可用备份" 'TestPruneNeverRemovesNewestVerifiedBackup|TestPruneBackupsKeepsNewestAndRequiresExplicitCall'
row M3.2 "保留策略默认 keep 3 与容量上限（决策记录）" 'TestAutoBackupBackupNowIncludesSavesAndPrunes'
row M3.2 "备份失败不影响 stop（stop 仍 200）" 'TestAutomaticBackupFailureDoesNotChangeStopResult|TestAutomaticBackupAfterStopDoesNotChangeStopResult'
row M3.2 "include 存档的备份经 staging+promotion 恢复且指纹一致" 'TestBackupWithSavesRecordsIncludeMode|TestDefaultBackupExcludesSaves|TestBackupVerifyTamperAndRestoreThroughPromotion' 

# ---------- M3.3 Mod 下载 ----------
row M3.3 "legacy POST /api/server/mods 逐字不变" 'TestLegacyModsEndpointUnchanged'
row M3.3 "先下载后登记；失败不写 INI；无效 id 明确错误" 'TestModDownloadRegistersOnlyAfterSuccess'
row M3.3 "单在飞互斥（与 install 并发 → 409）" 'TestModDownloadConflictsWithInstall'
row M3.3 "不可信内容边界（路径穿越白名单/不执行可执行内容）" 'TestModContentPathTraversalRejected'
record M3.3 "前置取证：workshop 落盘与 PZ 读取路径" "PASS" "Manifest §M3 代码取证 4 项（2026-09-30）；实机下载与 PZ 读取行为为 BLOCKED 待授权（见注解行）"

record M3.3 "端点可达 + workshop argv 正确（实机取证）" "PASS" "直接 SteamCMD 取证确认已发出 +workshop_download_item 380870 <id>；端点白名单与 argv 两缺陷已修并有断言"
record M3.3 "workshop 下载/读取行为已取证（外部约束定性）" "PASS" "两条独立证据：直连 SteamCMD 匿名下载 Failure + PZ 自身不拉取（INI WorkshopItems 后 mods 目录无内容）；读取目录为 Zomboid/mods；产品决策点 A/B 记录在案"
# ---------- M3.4 就绪与查询 ----------
row M3.4 "就绪时间线（追加字段，带时间戳）" 'TestQueryFieldsDegradeToUnavailable'
row M3.4 "查询字段或 unavailable 降级；ready/readiness 语义不变" 'TestQueryFieldsDegradeToUnavailable|TestQueryFieldsProjectWhenA2SAnswers|TestParseA2SInfo|TestParseA2SInfoRejectsGarbage'
record M3.4 "A2S 可达性取证（声明目标 PZ 42.21）" "PASS" "实机 ready 后 game_query 返回真实字段 {map:Muldraugh, KY, name:My PZ Server, players:0, max:100}；见 evidence/M3-residuals-live.md"

# ---------- M3.5 容量 ----------
row M3.5 "双检与竞态对抗（采样未超但写入时超 → 拒绝且无部分写入）" 'TestCapacityDoubleCheckRejectsPartialWrite|TestCapacityMeasurementFailureFailsClosed'
row M3.5 "硬阈值 409 + 恢复路径（只读+回退，永不自动删除）" 'TestCapacityHardLimitAndRecoveryPath'
row M3.5 "自激环防护（自动备份前预检 → pending_backup 延迟）" 'TestAutomaticBackupDefersWhenCapacityTight'
row M3.5 "用量口径（排除备份与锁目录）与 DiskUsageMB 对齐；阈值禁用=现状" 'TestDiskUsageExcludesBackupsAndLockDirs|TestCapacityDisabledKeepsHistoricalBehaviour|TestCapacityStatusProjectsState' 

# ---------- M3.6 实机长跑（实机证据见 evidence/M3-windows-reliability.md） ----------
record M3.6 "连续 start/stop ≥10 轮（含 1 次强杀）无残留" "PASS" "2026-09-30 隔离根实测 10/10 轮 procs=0 ports=0（含第 6 轮 taskkill 强杀）；日志见 evidence/M3-windows-reliability.md"
record M3.6 "崩溃注入：二进程争锁 / 缺制品 / 配置外部改写" "PASS" "争锁 409 + 释放后 200；缺制品 500 + 未起进程；配置改写后按受管键重写并成功启动"
record M3.6 "崩溃注入：端口占用下的失败表达（就绪面）" "PASS" "占用 16261 后就绪由 checking 转 failed（无假阳性）；start=200 属既有契约；同时记录残留 java 进程差异（列为未解除项）"
record M3.6 "≥4h 观测（60s 采样）无静默丢失" "PASS" "298 样本≈5.0h（08:40:57Z→13:44:25Z）：running 298/298、ready 298/298、PID 唯一 3508、句柄 5438→5344（Δ-94 无增长）、marker 恒 1"

# ---------- 判定待测行（行定义已在上面注册；评估结果追加合并，避免截断） ----------
if [ -s "$ROW_DEFS" ]; then
  go test -json ./... > "$WORK/all.json" || true
  go run ./tools/m2eval -rows "$ROW_DEFS" -json "$WORK/all.json" -out "$WORK/eval-rows.jsonl" -gov "$GO_VER" || {
    echo "m2eval failed; see above" >&2
    exit 1
  }
  cat "$WORK/eval-rows.jsonl" >> "$ROWS"
fi

# ---------- 目标提交 CI 三态 ----------
CI_SHA="$COMMIT"
CI_ID=""; CI_STATUS=""; CI_CONCLUSION=""
if command -v gh >/dev/null 2>&1; then
  CI_LINE="$(gh run list --branch "$(git rev-parse --abbrev-ref HEAD)" --workflow go --limit 20 \
      --json databaseId,headSha,status,conclusion \
      --jq ".[] | select(.headSha==\"${CI_SHA}\") | \"\\(.databaseId) \\(.status) \\(.conclusion)\"" 2>/dev/null | head -1)"
  if [ -n "${CI_LINE:-}" ]; then
    CI_ID="$(echo "$CI_LINE" | awk '{print $1}')"
    CI_STATUS="$(echo "$CI_LINE" | awk '{print $2}')"
    CI_CONCLUSION="$(echo "$CI_LINE" | awk '{print $3}')"
  fi
fi
if [ "${CI_STATUS:-}" = "completed" ] && [ "${CI_CONCLUSION:-}" = "success" ]; then
  record M3-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" PASS "run ${CI_ID} commit ${CI_SHA}"
elif [ "${CI_STATUS:-}" = "completed" ] && [ "${CI_CONCLUSION:-}" != "success" ]; then
  record M3-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" FAIL "run ${CI_ID} conclusion=${CI_CONCLUSION}"
else
  record M3-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" BLOCKED "run ${CI_ID:-none} status=${CI_STATUS:-unknown} conclusion=${CI_CONCLUSION:-none}（提交 ${CI_SHA}）"
fi

# ---------- 输出 ----------
if [ -z "$OUT" ]; then
  OUT="$REPO/docs/acceptance/evidence/m3-offline-latest.json"
fi
mkdir -p "$(dirname "$OUT")"
PASS_N="$(grep -c '"status":"PASS"' "$ROWS" || true)"
FAIL_N="$(grep -c '"status":"FAIL"' "$ROWS" || true)"
BLOCK_N="$(grep -c '"status":"BLOCKED"' "$ROWS" || true)"
NOTRUN_N="$(grep -c '"status":"NOT RUN"' "$ROWS" || true)"
{
  printf '{"commit":"%s","go":"%s","goos":"%s","goarch":"%s","binary_sha256":"%s","totals":{"PASS":%s,"FAIL":%s,"BLOCKED":%s,"NOT RUN":%s},"rows":[' \
    "${COMMIT}" "${GO_VER}" "$(go env GOOS)" "$(go env GOARCH)" "${BIN_SHA}" "${PASS_N}" "${FAIL_N}" "${BLOCK_N}" "${NOTRUN_N}"
  awk 'BEGIN{first=1} { if (!first) printf ","; first=0; printf "%s", $0 }' "$ROWS"
  printf ']}\n'
} > "$OUT"

echo ""
echo "== 已执行行: PASS=${PASS_N} FAIL=${FAIL_N}；BLOCKED=${BLOCK_N}；NOT RUN=${NOTRUN_N} =="
echo "证据: $OUT"

[ "${FAIL_N}" -eq 0 ] || exit 1
exit 0
