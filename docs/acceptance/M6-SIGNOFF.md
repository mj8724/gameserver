# M6 签核：认证远程节点（2026-09-30）

> 口径：隔离数据根基线验收；平台声明仅 Windows 10 x64。

## 1. 计数

- `m4m6-offline.sh` → exit 0；M6 相关行：**PASS = 7 / FAIL = 0 / BLOCKED = 0 / NOT RUN = 0**
  - M6.1 节点身份 3 项 + 幂等台账 4 项 + **断线对账 2 项** PASS
  - M6.2 远程边界 4 项 + 审计轨迹 3 项 PASS
  - M6.2 节点升级/回滚**离线演练** PASS（轮换→重钉指纹→回滚、身份不可转移）；安全审查见 `M6-security-review.md`
  - M6.2 双节点实机演练（注册/远程执行/回放/冲突/跨实例拒绝/对账端点）PASS；**rotate→重钉→回滚**在终批实机复验（见 M3-residuals-live.md §7）

## 2. 交付

| 能力 | 实现 | 证据 |
|---|---|---|
| 节点身份（ed25519、0600、轮换保 id） | `internal/adapters/nodestate` | `TestIdentityKeyFileIsPrivate`、`TestRotateChangesKeyKeepsNodeID` |
| 对等注册/撤销/失败关闭授权 | 同上 | `TestAuthorizeFailsClosed` |
| 持久幂等任务（重放/冲突/终态不可覆盖/跨重启/时钟漂移） | `ports.TaskLedger` + `nodestate.Ledger` | 4 项断言 |
| **断线对账** | `ports.TaskReconciler` + `Ledger.Reconcile`（pending 超宽限→`interrupted` 终态；启动时自动对账；`POST /api/tasks/reconcile` 可显式触发；状态持久） | `TestLedgerReconcileClosesDisconnectedTask`、`TestLedgerReconcileIsDurable` |
| 远程执行边界（封闭操作集、无任意命令） | `internal/application/remote.go` | `TestRemoteRejectsArbitraryOperation` |
| 授权 + 回放 | 同上 | `TestRemoteUnauthorizedNodeIsAudited`、`TestRemoteReplayDoesNotExecuteAgain` |
| 审计（追加式、0600、损坏失败关闭、全路径记录） | `internal/adapters/auditlog` | `TestRemoteAuditEntryIsComplete`、`TestAuditAppendOnly`、`TestAuditRecentLimitAndCorruption` |
| 安全审查 | OWASP/STRIDE 维度对照 + 未覆盖项 | `M6-security-review.md` |

## 3. 边界与后续项

1. **跨进程 rotate/re-pin/回滚已完成并修正顺序**：`M3-residuals-live.md §8.1` 记录 PIN_V1 → ROTATED → PIN_V2 → **STALE_FP1_AFTER_REPIN=节点未获授权** → FRESH_FP2 可用 → 回滚 PIN_V1 可用 → REVOKE 后拒绝；首轮"陈旧指纹被拒"断言顺序写反的脚本缺陷已修正。
2. 传输层认证（mTLS）、RBAC、审计外发/防篡改后端：明确未启用（产品化前置，见安全审查 §3）。

## 4. 结论

M6 的身份、幂等、断线对账、远程边界与审计在离线判定、**双节点实机演练**（注册/远程执行/回放/冲突/跨实例拒绝/对账端点）与**跨进程 rotate→重钉→回滚→撤销**（`M3-residuals-live.md §8.1`）下全部通过；安全审查见 `M6-security-review.md`（其未覆盖项为**产品化前置**：mTLS/RBAC/审计外发，非本里程碑交付项）。
