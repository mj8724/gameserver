# M6 安全审查（远程执行、身份、审计）

> 范围：M6.1 节点身份/幂等台账 + M6.2 远程执行边界与审计。方法：对**已实现代码**做 STRIDE/OWASP 维度的逐项对照（非假设性清单）。
> 结论口径：**非独立复核**（teammate 额度受限，ADR §11.3 豁免留痕）。

## 1. 威胁维度对照

| 维度 | 已实现的控制 | 证据（代码/测试） |
|---|---|---|
| **S**poofing 身份伪造 | 节点身份为 ed25519 密钥对；对等方按 `node_id + fingerprint` 授权，指纹不匹配即拒绝；远程执行的操作者取自**已认证会话**，请求体不能自称他人 | `nodestate.Store.Authorize`、`TestAuthorizeFailsClosed`、`httpapi.sessionOperator` |
| **T**ampering 篡改 | 节点存储/台账/注册表一律 `temp+fsync+rename+读回逐字节校验`，文件 0600；审计轨迹**只追加**（`O_APPEND` + `Sync`），不重写不截断 | `nodestate.writeVerified`、`auditlog.Log.Append`、`TestAuditAppendOnly`、`TestLedgerSurvivesRestartAndClockSkew` |
| **R**epudiation 抵赖 | 每次远程尝试都落审计：`operator/node/operation/request_id/input_hash/outcome/at`（含拒绝、未授权、冲突、回放、执行、失败六类结果） | `ControlService.audit`、`TestRemoteAuditEntryIsComplete` |
| **I**nformation disclosure 信息泄露 | 私钥路径与私钥材料**永不返回**（`GET /api/nodes` 只回 `node_id/fingerprint/public_key/created_at/rotated_at`）；`GET /api/audit` 只回审计字段 | `httpapi.nodes`、ADR §2.3 r6 记录 |
| **D**enial of service | 远程操作受封闭白名单限制，无任意命令通道；幂等台账阻止重放放大；审计损坏**失败关闭**（不静默跳过） | `remoteAllowlist`、`TaskLedger.Begin`、`TestAuditRecentLimitAndCorruption` |
| **E**levation of privilege 越权 | 所有远程/节点/实例端点先过 `requireSession`；实例注销拒绝活跃实例自注销；跨实例操作由各自进程的实例绑定隔离（M5.2 实测） | `requireSession`、`TestRemoveInstanceRefusesActive`、`M5-dual-instance.md` |

## 2. 重放与断线（M6 核心要求）

- **重放**：同 `request_id` + 同输入指纹 → 返回既有终态且 `Executed=false`（不再执行）；同 id 异输入 → `ErrTaskConflict` 且原记录不被覆盖；终态 `Complete` 幂等（二次完成不覆盖结果）。
- **断线**：台账落盘（读回校验），进程重启后仍可 `Get`；完成记录不会被后续重放改写。
- **时钟漂移**：记录接受早于当前时间的 `CreatedAt`，不影响排序与判定。

## 3. 未覆盖 / 保留（如实记录）

| 项 | 状态 | 说明 |
|---|---|---|
| 节点升级与回滚演练 | **已覆盖** | 双节点实机：`PIN_V1 → ROTATED → PIN_V2 → STALE_FP1 被拒 → FRESH_FP2 可用 → 回滚 PIN_V1 可用 → REVOKE 后拒绝`（`M3-residuals-live.md §8.1`）；离线状态机断言 `TestNodeUpgradeRollbackDrill`、`TestNodeIdentityIsNotTransferable`。跨主机（独立机型）演练仍可作为产品化增强项 |
| 传输层认证（mTLS/传输加密） | 未启用 | 当前节点间以指纹授权为主，传输加密依赖部署层（反代/隧道）；列为产品化前置 |
| RBAC/多操作者区分 | 未启用 | 计划明确保留（`operator` 目前为管理员会话标识） |
| 审计日志外发/防篡改后端 | 未启用 | 目前为本地追加式 JSONL + fsync；防篡改归档需产品决策 |

## 4. 结论

M6 的安全控制在**已实现范围内**满足：身份不可伪造（指纹绑定）、记录不可抵赖（全路径审计）、重放不重复执行（含断线对账后的回放）、越权被拒、私钥不泄露、审计失败关闭；**升级/回滚与撤销**已由双节点实机演练覆盖。**传输层加密（mTLS）、RBAC、审计外发/防篡改后端**为**产品化前置的非目标**（计划明确保留，需独立决策与安全审查），不属本里程碑交付缺口。
