# M6 签核：认证远程节点（2026-09-30）

> 口径：隔离数据根基线验收；平台声明仅 Windows 10 x64。

## 1. 计数

- `m4m6-offline.sh` → exit 0；M6 相关行：**PASS = 4 / FAIL = 0 / BLOCKED = 1 / NOT RUN = 0**
  - M6.1 节点身份 3 项 + 幂等台账 4 项 PASS
  - M6.2 远程边界 4 项 + 审计轨迹 3 项 PASS
  - M6.2 **节点升级/回滚演练与安全审查：BLOCKED**（安全审查已落文档；升级/回滚演练需第二节点）

## 2. 交付

| 能力 | 实现 | 证据 |
|---|---|---|
| 节点身份（ed25519、0600、轮换保 id） | `internal/adapters/nodestate` | `TestIdentityKeyFileIsPrivate`、`TestRotateChangesKeyKeepsNodeID` |
| 对等注册/撤销/失败关闭授权 | 同上 | `TestAuthorizeFailsClosed` |
| 持久幂等任务（重放/冲突/终态不可覆盖/跨重启/时钟漂移） | `ports.TaskLedger` + `nodestate.Ledger` | 4 项断言 |
| 远程执行边界（封闭操作集、无任意命令） | `internal/application/remote.go` | `TestRemoteRejectsArbitraryOperation` |
| 授权 + 回放 | 同上 | `TestRemoteUnauthorizedNodeIsAudited`、`TestRemoteReplayDoesNotExecuteAgain` |
| 审计（追加式、0600、损坏失败关闭、全路径记录） | `internal/adapters/auditlog` | `TestRemoteAuditEntryIsComplete`、`TestAuditAppendOnly`、`TestAuditRecentLimitAndCorruption` |
| 安全审查 | OWASP/STRIDE 维度对照 + 未覆盖项 | `M6-security-review.md` |

## 3. 未解除项

1. **节点升级/回滚演练**：需第二个节点实例（独立主机/容器），单机隔离根无法构成真实场景。
2. 传输层认证（mTLS）、RBAC、审计外发/防篡改后端：明确未启用（产品化前置，见安全审查 §3）。

## 4. 结论

M6 的身份、幂等、远程边界与审计在**已实现范围内**通过离线判定与安全审查；**节点升级/回滚演练为未解除项**。
