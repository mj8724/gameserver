#!/usr/bin/env bash
# M2 离线矩阵执行器（E-OFF / 本机 darwin 作为开发平台）
#
# 只执行 docs/acceptance/M2-GO-PZ-MVP.md 中不依赖真实 PZ、SteamCMD、远端主机、
# 浏览器或网络探测的矩阵行；其余行一律记为 BLOCKED 并说明原因，绝不记 PASS。
#
# 用法: bash docs/acceptance/rehearsals/m2-offline.sh [--out <json 路径>]
# 退出码: 0 = 所有已执行行通过；1 = 有已执行行 FAIL
set -u

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PATH="$HOME/.local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
cd "$REPO" || exit 1

OUT="${M2_OUT:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

WORK="$(mktemp -d)"
ROWS="$WORK/rows.jsonl"
: > "$ROWS"

PASS_N=0; FAIL_N=0; BLOCK_N=0

cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

record() { # id | title | status | evidence
  printf '{"id":"%s","title":"%s","status":"%s","evidence":"%s"}\n' "$1" "$2" "$3" "$4" >> "$ROWS"
  case "$3" in
    PASS) PASS_N=$((PASS_N+1)) ;;
    BLOCKED) BLOCK_N=$((BLOCK_N+1)) ;;
    *) FAIL_N=$((FAIL_N+1)) ;;
  esac
  printf '%-16s %-9s %s\n' "$1" "$3" "$2" >&2
}

# 行级判定：给定行 ID/标题/说明与测试选择器，运行 go test 并记 PASS/FAIL
ROW_DEFS="$WORK/rowdefs.tsv"
: > "$ROW_DEFS"
# 记录待判定行；测试只在下方单次运行，判定按测试名结果完成（避免 N 次全量重跑）
row() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$ROW_DEFS"; }

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

# ---------- 构建门（M2-BUILD #1：本机部分；CI 部分另记） ----------
BUILD_OK=1
test -z "$(gofmt -l cmd internal)" || BUILD_OK=0
go vet ./... >/dev/null 2>&1 || BUILD_OK=0
go build ./... >/dev/null 2>&1 || BUILD_OK=0
go mod tidy -diff >/dev/null 2>&1 || BUILD_OK=0
if [ "$BUILD_OK" -eq 1 ]; then
  record M2-BUILD "固定工具链与本机门禁（gofmt/vet/build/tidy）" PASS "go=$GO_VER commit=$COMMIT binary_sha256=$BIN_SHA"
else
  record M2-BUILD "固定工具链与本机门禁（gofmt/vet/build/tidy）" FAIL "见上方步骤输出"
fi

# ---------- 矩阵行 ----------
row M2-BUILD "archtest 直接导入边界 + 负向注入探针" 'InjectedForbiddenImportFailsArchitectureCheck|RulesRejectForbiddenImports|TestImportBoundaries'
row M2-API "20 挂载点/严格方法与错误 envelope" 'RouteTableCompatibilityAndStrictMethods|ApplicationErrorsAndConfigurationRedaction'
row M2-API "模板元数据与追加字段（ready/readiness）" 'ControlStatusProjectsStateAndSecrets|CatalogLoadsValidTemplates'
row M2-API "auth/Origin 边界完整性" 'AuthOriginAndRequestValidation|AuthenticatorAcceptsTokenAndRejectsTamperedSignature'
row M2-AUTH "登录/会话正常与失败路径" 'AuthenticatorRejectsMissingAndIncorrectConfiguredPassword|AuthenticatorTokenExpiresAtTwelveHours|AuthenticatorRejectsFutureAndExpiredTimestamps|AuthenticatorHTTPLoginLogoutCookieAndReplay'
row M2-AUTH "logout 重放与重启失效（D2）" 'AuthenticatorLogoutRevokesReplay|AuthenticatorRestartInvalidatesOldTokens'
row M2-AUTH "Origin/CSRF（D1）" 'AuthOriginAndRequestValidation'
row M2-WS "接受前认证与 Origin 门" 'ConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup'
row M2-WS "帧契约、回放与 D3 未运行输入" 'ConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup|ConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands'
row M2-WS "断连取消与 listener 清理" 'ConsoleServerCancellationClosesConnectionAndSubscription'
row M2-WS "读写超时与慢客户端隔离" 'ConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup|ConsoleServerCancellationClosesConnectionAndSubscription'
row M2-WS "错误帧 wire 契约（D3）" 'ConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup'
row M2-SINGLEWRITER "启动对账：无记录/残留进程/PID/指纹/token/root" 'ExistingRecordOrResidualProcessRequiresRecovery|RecoverClearsStaleRecordAndInspectionReportsState|AcquireWritesOwnershipOutsideInstanceTree'
row M2-SINGLEWRITER "未取得锁时的服务行为（跨进程）" 'SecondProcessIsRefusedAndReleaseFreesTheInstance|MigrationLockIsExclusive'
row M2-SINGLEWRITER "启动时存在非终态 migration journal" 'JournalIsValidJSONAndBlockedStatesAreNonTerminal'
row M2-SECRET "秘密脱敏（状态/日志/argv/指纹）" 'ControlStatusProjectsStateAndSecrets|ArgumentAndChildOutputLogsRedactAdminPassword|AdversarialArgvReachesHelperWithoutSplittingAndIsRedacted|BuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv'
row M2-SECRET "持久秘密数据边界" 'SaveAtomicBackupModesAndUnknownFieldPreservation|DefaultStateGeneratesSecretWithoutLogging'
row M2-INSTALL "安装成功/失败/冲突/取消/deadline/retry" 'ControlInstallLifecycleAndConflicts|InstallRunsTypedArgvParsesProgressAndClearsBusy|InstallFailureAndConflictHaveExitStatusAndClearBusy|InstallCancellationTerminatesWaitsAndClearsBusy|InstallDeadlineEscalatesToKillThenReaps'
row M2-CONFIG "JSON/INI 原子写 round-trip 与故障注入" 'SaveAtomicBackupModesAndUnknownFieldPreservation|AtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile|INIAtomicFailureDoesNotReturnSuccessAndPreservesOldOrNew'
row M2-CONFIG "INI/状态兼容与未知键保留" 'INIReadWritePreservesCommentsUnknownKeysAndOnlyManagedChanges|LoadImportsLegacyAndMergesDefaultsWithoutWriting|ManagedINIUpdatesMapsOnlyManagedKeys'
row M2-CONFIG "配置校验边界与可编辑字段" 'ValidateConfigUpdateEnforcesDeclaredFieldsTypesAndRanges|ControlConfigValidationAndConsoleInput|INIValidationRejectsTraversalUnknownKeysAndLineInjection'
row M2-CONFIG "target/backup 损坏与失败关闭" 'LoadInvalidServerNameFallsBackAndCorruptStateFailsClosed|BackupVerifyTamperAndRestoreThroughPromotion'
row M2-MIGRATE "ADR §4.3 恢复表全部现场" 'RecoveryTableRows|DryRunWritesNothing|ImportThenPromoteKeepsPrevAndLeavesOtherTreesAlone|FirstPromotionHasNoPrev'
row M2-MIGRATE "中断注入：步骤1/步骤2/双失败/前滚校验和" 'StepOneFailureIsRecoveryRequiredAndDeletesNothing|StepTwoFailureRollsBack|StepTwoAndRollbackFailureIsRecoveryRequired|CommittedForwardRollRequiresMatchingChecksum'
row M2-MIGRATE "备份/损坏拒绝/恢复全链路 + prune" 'BackupVerifyTamperAndRestoreThroughPromotion|PruneBackupsKeepsNewestAndRequiresExplicitCall'
row M2-PROCESS "进程生命周期 helper 与对抗 argv/停止" 'StartFailureAndFastExitDoNotReportSuccess|InputGracefulStopAndIdempotentKillReapChild|StopTimeoutKillsAndReapsIgnoringHelper|LogBufferIsBoundedToOneThousandNewestLines'
row M2-PROCESS "进程状态与日志上限" 'LogLimitIsClampedAndStatusDoesNotExposeSecrets|WrongInstanceAndCancelledStartAreRejected'
row M2-PROCESS "就绪探测（SERVER_PORT/超时/取消/marker）" 'ReadinessUsesSERVERPORTAndInjectedProbeAndTimeout|ReadinessRequiresManifestMarkerAndRunningProcess|ReadinessPropagatesCancellation'
row M2-API "静态资源安全回退" 'StaticAssetsFallbackAndSafePaths|OpenRejectsTraversalAndAbsoluteNames|OpenRejectsSymlinkEscape'

# ---------- CI 门：目标提交的 GitHub Actions 结果 ----------
# 判定为三态，保证脚本可重复执行且不被 CI 时序误判：
#   success            → PASS
#   失败/取消/超时     → FAIL
#   运行中/排队/无运行 → BLOCKED（另行记录 run id 与提交）
CI_SHA="$(git rev-parse HEAD)"
CI_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
CI_ROW="$(gh run list --branch "$CI_BRANCH" --workflow go --limit 40 --json databaseId,headSha,conclusion,status,url \
  --jq "[.[] | select(.headSha==\"$CI_SHA\")][0] | \"\(.databaseId) \(.conclusion) \(.status) \(.url)\"" 2>/dev/null || true)"
# 只取第一行、按字段解析；未知/空值一律 BLOCKED，只有明确的失败结论才记 FAIL。
CI_LINE="$(printf '%s\n' "$CI_ROW" | head -1 | tr -d '\r')"
CI_FIELDS="$(printf '%s' "$CI_LINE" | awk '{print NF}')"
CI_ID="$(printf '%s' "$CI_LINE" | awk '{print $1}')"
CI_CONCLUSION="$(printf '%s' "$CI_LINE" | awk '{print $2}')"
CI_STATUS="$(printf '%s' "$CI_LINE" | awk '{print $3}')"
CI_URL="$(printf '%s' "$CI_LINE" | awk '{print $4}')"
case "$CI_CONCLUSION" in
  success)
    if [ "${CI_FIELDS:-0}" -ge 4 ]; then
      record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" PASS "run $CI_ID success for $CI_SHA $CI_URL"
    else
      record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" BLOCKED "CI 输出字段不完整（提交 ${CI_SHA}）"
    fi ;;
  failure|cancelled|timed_out|startup_failure|stale)
    record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" FAIL "run $CI_ID conclusion=$CI_CONCLUSION ($CI_SHA)" ;;
  *)
    record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" BLOCKED "run ${CI_ID:-none} status=${CI_STATUS:-unknown} conclusion=${CI_CONCLUSION:-none}（提交 ${CI_SHA}）" ;;
esac

# 真实装配/源码级断言（评估前登记，供单次测试运行后统一判定）
row M2-API "静态 UI 调用面与路由表一致性（源码级）" 'TestStaticUIContract'
row M2-UI "未登录/会话过期界面分支（源码级：精确文案分支）" 'TestStaticUIContract'
row M2-WS "D3 error frame 不被渲染为日志（源码级 onmessage 断言）" 'TestStaticUIContract'
row M2-RESTART "陈旧 owner 记录 → 失败关闭、不自动清理、只读可用" 'TestM2OfflineRecoveryRequiredReconciliation'
row M2-SINGLEWRITER "陈旧 owner 记录对账（darwin 补充证据）" 'TestM2OfflineRecoveryRequiredReconciliation'
row M2-PROCESS "环境变量与模板插值隔离（darwin 补充证据）" 'BuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv|BuildLaunchSpecValidatesNamesAndNeverAcceptsTemplate'
row M2-API "真实装配黑盒：路由/错误文案/投影/落盘/权限/锁栅栏" 'TestM2OfflineHTTPBlackBox'
row M2-CONFIG "D8 权限门失败关闭 + 归一后成功（darwin 补充证据）" 'TestM2OfflineHTTPBlackBox'
row M2-RESTART "重启后读取已提交状态且对账干净（离线部分）" 'TestM2OfflineRestartPersistence'
row M2-SECRET "口令值不回显（状态与配置投影）" 'TestM2OfflineHTTPBlackBox'

row M2-PROCESS "Windows 进程树终止 argv（类型化、无 shell；运行仍需目标机）" 'TestTaskkillArgsStayTypedAndExact'
row M2-INSTALL "SteamCMD 安装器配置解析与未配置时失败关闭" 'TestSteamcmdInstallConfigResolution|TestRuntimeConfiguresInstallerFromTemplate'

row M2-INSTALL "端到端：install → is_installed → start → command → stop（真实 HTTP/装配，注入 fake 游戏适配器）" 'TestM2OfflineInstallStartStopLifecycle'
row M2-RESTART "端到端停止后状态枚举与 running 语义" 'TestM2OfflineInstallStartStopLifecycle'

# ---------- 单次运行全部 Go 测试并按行判定 ----------
go test -count=1 -json ./... > "$WORK/all.json" 2>"$WORK/all.err" || true
go run ./tools/m2eval -rows "$ROW_DEFS" -json "$WORK/all.json" -out "$ROWS" -gov "$(go version | awk '{print $3}')" || {
  echo "m2eval failed; see $WORK/all.err" >&2
  exit 1
}
grep -o '"id":"[^"]*","title":"[^"]*","status":"[^"]*"' "$ROWS" | sed 's/"id":"//; s/","title":"/ | /; s/","status":"/ | /' >&2

# ---------- CI 门：目标提交的 GitHub Actions 结果 ----------
# 判定为三态，保证脚本可重复执行且不被 CI 时序误判：
#   success            → PASS
#   失败/取消/超时     → FAIL
#   运行中/排队/无运行 → BLOCKED（另行记录 run id 与提交）
CI_SHA="$(git rev-parse HEAD)"
CI_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
CI_ROW="$(gh run list --branch "$CI_BRANCH" --workflow go --limit 40 --json databaseId,headSha,conclusion,status,url \
  --jq "[.[] | select(.headSha==\"$CI_SHA\")][0] | \"\(.databaseId) \(.conclusion) \(.status) \(.url)\"" 2>/dev/null || true)"
# 只取第一行、按字段解析；未知/空值一律 BLOCKED，只有明确的失败结论才记 FAIL。
CI_LINE="$(printf '%s\n' "$CI_ROW" | head -1 | tr -d '\r')"
CI_FIELDS="$(printf '%s' "$CI_LINE" | awk '{print NF}')"
CI_ID="$(printf '%s' "$CI_LINE" | awk '{print $1}')"
CI_CONCLUSION="$(printf '%s' "$CI_LINE" | awk '{print $2}')"
CI_STATUS="$(printf '%s' "$CI_LINE" | awk '{print $3}')"
CI_URL="$(printf '%s' "$CI_LINE" | awk '{print $4}')"
case "$CI_CONCLUSION" in
  success)
    if [ "${CI_FIELDS:-0}" -ge 4 ]; then
      record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" PASS "run $CI_ID success for $CI_SHA $CI_URL"
    else
      record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" BLOCKED "CI 输出字段不完整（提交 ${CI_SHA}）"
    fi ;;
  failure|cancelled|timed_out|startup_failure|stale)
    record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" FAIL "run $CI_ID conclusion=$CI_CONCLUSION ($CI_SHA)" ;;
  *)
    record M2-BUILD "目标提交 GitHub CI 全绿（含 race 与 archtest）" BLOCKED "run ${CI_ID:-none} status=${CI_STATUS:-unknown} conclusion=${CI_CONCLUSION:-none}（提交 ${CI_SHA}）" ;;
esac

# 真实装配/源码级断言（评估前登记，供单次测试运行后统一判定）
row M2-API "静态 UI 调用面与路由表一致性（源码级）" 'TestStaticUIContract'
row M2-UI "未登录/会话过期界面分支（源码级：精确文案分支）" 'TestStaticUIContract'
row M2-WS "D3 error frame 不被渲染为日志（源码级 onmessage 断言）" 'TestStaticUIContract'
row M2-RESTART "陈旧 owner 记录 → 失败关闭、不自动清理、只读可用" 'TestM2OfflineRecoveryRequiredReconciliation'
row M2-SINGLEWRITER "陈旧 owner 记录对账（darwin 补充证据）" 'TestM2OfflineRecoveryRequiredReconciliation'
row M2-PROCESS "环境变量与模板插值隔离（darwin 补充证据）" 'BuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv|BuildLaunchSpecValidatesNamesAndNeverAcceptsTemplate'
row M2-API "真实装配黑盒：路由/错误文案/投影/落盘/权限/锁栅栏" 'TestM2OfflineHTTPBlackBox'
row M2-CONFIG "D8 权限门失败关闭 + 归一后成功（darwin 补充证据）" 'TestM2OfflineHTTPBlackBox'
row M2-RESTART "重启后读取已提交状态且对账干净（离线部分）" 'TestM2OfflineRestartPersistence'
row M2-SECRET "口令值不回显（状态与配置投影）" 'TestM2OfflineHTTPBlackBox'

row M2-PROCESS "Windows 进程树终止 argv（类型化、无 shell；运行仍需目标机）" 'TestTaskkillArgsStayTypedAndExact'
row M2-INSTALL "SteamCMD 安装器配置解析与未配置时失败关闭" 'TestSteamcmdInstallConfigResolution|TestRuntimeConfiguresInstallerFromTemplate'

row M2-INSTALL "端到端：install → is_installed → start → command → stop（真实 HTTP/装配，注入 fake 游戏适配器）" 'TestM2OfflineInstallStartStopLifecycle'
row M2-RESTART "端到端停止后状态枚举与 running 语义" 'TestM2OfflineInstallStartStopLifecycle'

# ---------- 单次运行全部 Go 测试并按行判定 ----------
go test -count=1 -json ./... > "$WORK/all.json" 2>"$WORK/all.err" || true
python3 - "$ROW_DEFS" "$WORK/all.json" "$ROWS" "$(go version | awk '{print $3}')" <<'PYEVAL'
import json, re, sys
defs_path, json_path, rows_path, gov = sys.argv[1:5]
status, seen = {}, set()
for line in open(json_path):
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        event = json.loads(line)
    except json.JSONDecodeError:
        continue
    name = event.get("Test")
    if not name:
        continue
    seen.add(name)
    action = event.get("Action")
    if action == "fail":
        status[name] = "fail"
    elif action == "pass" and status.get(name) != "fail":
        status[name] = "pass"
    elif action == "skip" and name not in status:
        status[name] = "skip"

with open(rows_path, "a") as out:
    for line in open(defs_path):
        if not line.strip():
            continue
        rid, title, selector = line.rstrip("\n").split("\t", 2)
        pattern = re.compile(selector)
        matched = [n for n in sorted(seen) if pattern.search(n)]
        failed = [n for n in matched if status.get(n) == "fail"]
        if not matched:
            state, evidence = "FAIL", "选择器未匹配到任何测试（%s）" % selector
        elif failed:
            state, evidence = "FAIL", "失败用例: " + ", ".join(failed[:4])
        else:
            state = "PASS"
            evidence = "%d 个用例全通过（E-OFF, %s）: %s" % (
                len(matched), gov, ", ".join(matched[:3]) + ("…" if len(matched) > 3 else ""))
        out.write(json.dumps({"id": rid, "title": title, "status": state, "evidence": evidence},
                             ensure_ascii=False) + "\n")
PYEVAL
python3 - "$ROWS" <<'PYSHOW'
import json, sys
for line in open(sys.argv[1]):
    if line.strip():
        r = json.loads(line)
        print("%-16s %-9s %s" % (r["id"], r["status"], r["title"]), file=sys.stderr)
PYSHOW

# 真实装配的黑盒断言以 Go 集成测试执行（计划 §1.2：优先 httptest、不绑定端口）：
# 真实 state store + PZ INI 适配器 + 模板目录 + 真实 handler，仅 listener 由 httptest 注入。

# ---------- BLOCKED 行（不执行，必须显式记录原因） ----------
b() { record "$1" "$2" BLOCKED "$3"; }
b M2-UI "静态 UI 登录/仪表盘浏览器闭环" "需要真实浏览器与人工操作（E-BROWSER），本机离线不执行"
b M2-UI "未登录/会话过期界面分支（浏览器验证）" "源码级分支与精确文案已由 TestStaticUIContract 验证；浏览器中的人工闭环仍需 E-BROWSER"
b M2-WS "D3 error frame 浏览器互操作（UI 稳态）" "源码级断言已确认 error 帧不进入日志渲染；浏览器 console 无异常的观察仍需 E-BROWSER"
b M2-SECRET "POSIX 秘密目录/文件权限（Linux 目标机）" "需 Linux/Ubuntu LTS 目标主机；darwin 证据仅作补充"
b M2-CONFIG "变更操作的 D8 权限门（Linux）" "需 Linux 目标主机"
b M2-MIGRATE "staging 边界、同卷与锁（目标文件系统）" "需目标文件系统与授权的一次性根"
b M2-MIGRATE "仅因权限宽松拒绝提升（Linux）" "需 Linux 目标主机与逐项写权限授权"
b M2-INSTALL "正常结束/取消后的 reap（声明平台）" "darwin 已用真实 helper 子进程覆盖；Linux 声明仍需目标主机"
b M2-PROCESS "Linux/Windows 启动向量对抗" "需具名目标 OS 与已编译 safe helper；Manifest 未填"
b M2-PROCESS "环境变量与模板插值隔离（声明平台 helper 实测）" "darwin 已用包内测试覆盖构造成本；声明平台仍需具名 runner 与 safe helper"
b M2-SINGLEWRITER "双独立进程争用（Linux 变体）" "Windows 已在目标机实测（M2-windows-two-process.md：409/只读 200/无写入/强杀后 recovery required/recover 后恢复）；Linux 变体仍需具名主机"
b M2-RESTART "控制进程异常退出后的启动对账（E-OS 声明）" "darwin 已由 TestM2OfflineRecoveryRequiredReconciliation 覆盖；目标 OS 声明仍需具名 runner"
b M2-RESTART "PZ child 意外退出观察（E-OS）" "需真实/受控子进程与目标 OS"
b M2-PZ-LIVE "全部 7 行" "需 Target Manifest、用户逐项授权与真实 SteamCMD/PZ（BLOCKED）"
b M2-PLATFORM "全部 3 行" "需 Linux 目标主机资格与平台决策证据"

# ---------- 输出 ----------
if [ -z "$OUT" ]; then
  OUT="$REPO/docs/acceptance/evidence/m2-offline-latest.json"
fi
mkdir -p "$(dirname "$OUT")"
PASS_N="$(grep -c '"status":"PASS"' "$ROWS" || true)"
FAIL_N="$(grep -c '"status":"FAIL"' "$ROWS" || true)"
BLOCK_N="$(grep -c '"status":"BLOCKED"' "$ROWS" || true)"
{
  printf '{"commit":"%s","go":"%s","goos":"%s","goarch":"%s","binary_sha256":"%s","totals":{"PASS":%s,"FAIL":%s,"BLOCKED":%s},"rows":[' \
    "${COMMIT}" "${GO_VER}" "$(go env GOOS)" "$(go env GOARCH)" "${BIN_SHA}" "${PASS_N}" "${FAIL_N}" "${BLOCK_N}"
  awk 'BEGIN{first=1} { if (!first) printf ","; first=0; printf "%s", $0 }' "$ROWS"
  printf ']}\n'
} > "$OUT"

echo ""
echo "== 已执行行: PASS=${PASS_N} FAIL=${FAIL_N}；BLOCKED=${BLOCK_N} =="
echo "证据: $OUT"

[ "${FAIL_N}" -eq 0 ] || exit 1
exit 0
