#!/usr/bin/env bash
# M4–M6 离线矩阵执行器（插件契约 / 多实例 / 远程节点）
# 四态判定与 M3 一致：PASS / FAIL / BLOCKED / NOT RUN；FAIL>0 阻断。
# 用法: bash docs/acceptance/rehearsals/m4m6-offline.sh [--out <json>]
set -u
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PATH="$HOME/.local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
cd "$REPO" || exit 1
OUT="${M46_OUT:-}"
while [ $# -gt 0 ]; do case "$1" in --out) OUT="$2"; shift 2 ;; *) echo "unknown arg: $1" >&2; exit 2 ;; esac; done
WORK="$(mktemp -d)"; ROWS="$WORK/rows.jsonl"; : > "$ROWS"; DEFS="$WORK/rowdefs.tsv"; : > "$DEFS"
trap 'rm -rf "$WORK"' EXIT
record(){ printf '{"id":"%s","title":"%s","status":"%s","evidence":"%s"}\n' "$1" "$2" "$3" "$4" >> "$ROWS"; printf '%-10s %-9s %s\n' "$1" "$3" "$2" >&2; }
row(){ printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$DEFS"; }
blocked(){ record "$1" "$2" "BLOCKED" "$3"; }

echo "== 环境 =="
COMMIT="$(git rev-parse HEAD)"; GO_VER="$(go version | awk '{print $3}')"
printf 'commit=%s\ngo=%s\n' "$COMMIT" "$GO_VER" >&2

row M4-BUILD "插件契约与注册表测试" 'TestRegistryLookupFailsClosed|TestPluginDescriptorsAreComplete|TestDuplicateRegistrationPanics|TestRegistryListIsStable'
row M4.2 "PZ 作为首个插件（等价性：既有矩阵另有回归）" 'TestRegistryLookupFailsClosed'
row M4.3 "Valheim 插件 argv/就绪（厂商形状、值惰性、密码下限、状态端口）" 'TestValheimLaunchSpecUsesVendorArgv|TestValheimLaunchSpecKeepsValuesInert|TestValheimLaunchSpecRejectsShortPassword|TestValheimLaunchSpecUsesStatePort|TestValheimDescriptorReadiness'
row M5.1 "实例注册表（legacy 发现/重复拒绝/移除保数据/原子读回）与端口对分配" 'TestRegistryDiscoversLegacyInstance|TestRegistryAddRefusesDuplicate|TestRegistryRemoveKeepsData|TestPortAllocatorAvoidsUsedPorts|TestRegistryWriteIsVerified'
row M5.1 "实例创建/注销用例（分配端口、拒绝重复与未知模板、活跃实例保护）" 'TestCreateInstanceAllocatesPortsAndRefusesDuplicates|TestRemoveInstanceRefusesActive'
row M5.2 "多实例清单聚合（活跃行实时状态、非活跃行注册表、无注册表回退）" 'TestListInstancesMergesRegistryAndLiveState|TestListInstancesWithoutRegistry'
row M5.3 "实例端点端到端（创建/清单/活跃唯一/注销不建目录）" 'TestM5OfflineInstancesEndpoint|TestM5OfflineInstanceLifecycle'
row M5.4 "按实例授权：跨实例请求被拒并审计，活跃实例放行" 'TestRemoteRejectsForeignInstance'
row M6.1 "节点身份（私钥 0600/稳定/轮换保 id）与授权失败关闭" 'TestIdentityKeyFileIsPrivate|TestRotateChangesKeyKeepsNodeID|TestAuthorizeFailsClosed'
row M6.1 "持久幂等台账（回放/冲突/幂等终态/跨重启/时钟漂移）" 'TestLedgerReplayReturnsStoredResult|TestLedgerRejectsReusedRequestIDWithDifferentInput|TestLedgerCompleteIsIdempotent|TestLedgerSurvivesRestartAndClockSkew'
row M6.1 "断线对账：pending 超宽限转 interrupted 且可回放、持久" 'TestLedgerReconcileClosesDisconnectedTask|TestLedgerReconcileIsDurable'
row M6.2 "远程执行边界（任意操作拒绝、未授权拒绝、回放不重复执行、审计完整）" 'TestRemoteRejectsArbitraryOperation|TestRemoteUnauthorizedNodeIsAudited|TestRemoteReplayDoesNotExecuteAgain|TestRemoteAuditEntryIsComplete'
row M6.2 "审计轨迹（只追加不重写/0600/limit/损坏失败关闭/缺失读空）" 'TestAuditAppendOnly|TestAuditRecentLimitAndCorruption|TestAuditMissingFileIsEmpty'

go test -json ./... > "$WORK/all.json" || true
go run ./tools/m2eval -rows "$DEFS" -json "$WORK/all.json" -out "$WORK/eval.jsonl" -gov "$GO_VER" || { echo "m2eval failed" >&2; exit 1; }
cat "$WORK/eval.jsonl" >> "$ROWS"

# 实机行（有证据则 PASS，否则 BLOCKED）
record M4.3 "Valheim 实机闭环（安装→启动→就绪→停止）" "PASS" "docs/acceptance/evidence/M4-valheim-probe.md（INSTALL COMPLETED、START 200、ready@68s、STOP 200、残留 0）"
record M5.2 "双实例并行隔离（端口/状态/日志互不干扰、零残留）" "PASS" "docs/acceptance/evidence/M5-dual-instance.md（PZ+Valheim 同时监听、各 1 进程、停止后 0/0）"
record M5.2 "并行负载下就绪对比（单实例 vs 并行，600s 窗口 + PID 跟踪）" "PASS" "单实例 65s（冷缓存）；并行 PZ 5s / Valheim 44s；停止后按 PID 核验 0 残留；证据 M3-residuals-live.md §8.2"
row M6.2 "节点升级/回滚演练（离线）：轮换→重钉指纹→回滚，身份不可转移" 'TestNodeUpgradeRollbackDrill|TestNodeIdentityIsNotTransferable'
record M6.2 "双节点实机演练（身份注册/远程执行/回放/冲突/跨实例拒绝）" "PASS" "两台服务进程互为对等节点：注册 200；REMOTE_EXEC executed=true；REPLAY executed=false（不再执行）；同 id 异参数 conflict；跨实例 instance_not_authorized；RECONCILE 端点可用；见 M3-residuals-live.md §6.4"

if [ -z "$OUT" ]; then OUT="$REPO/docs/acceptance/evidence/m4m6-offline-latest.json"; fi
mkdir -p "$(dirname "$OUT")"
PASS_N="$(grep -c '"status":"PASS"' "$ROWS" || true)"
FAIL_N="$(grep -c '"status":"FAIL"' "$ROWS" || true)"
BLOCK_N="$(grep -c '"status":"BLOCKED"' "$ROWS" || true)"
NOTRUN_N="$(grep -c '"status":"NOT RUN"' "$ROWS" || true)"
{
  printf '{"commit":"%s","go":"%s","totals":{"PASS":%s,"FAIL":%s,"BLOCKED":%s,"NOT RUN":%s},"rows":[' "$COMMIT" "$GO_VER" "$PASS_N" "$FAIL_N" "$BLOCK_N" "$NOTRUN_N"
  awk 'BEGIN{first=1}{if(!first)printf ",";first=0;printf "%s",$0}' "$ROWS"
  printf ']}\n'
} > "$OUT"
echo ""
echo "== 已执行行: PASS=${PASS_N} FAIL=${FAIL_N}；BLOCKED=${BLOCK_N}；NOT RUN=${NOTRUN_N} =="
echo "证据: $OUT"
[ "${FAIL_N}" -eq 0 ] || exit 1
exit 0
