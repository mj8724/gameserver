# M3 签核（单实例可靠性，2026-09-30）

> 证据口径：**隔离数据根基线验收**（非生产路径；ADR §11.3 r5 / ROADMAP rev3）。
> 平台声明边界：**仅 Windows 10 x64**（声明对象，ADR §1.6 r4/r5）；**Linux/macOS 未验证**，本文件不改变该状态。
> 独立审查：**非独立复核**（teammate 提供商额度受限；豁免留痕见 ADR §11.3，补做触发=额度恢复或指定替代渠道）。

## 1. 计数（与证据 JSON 一致，先跑脚本后写数字）

- 证据：`docs/acceptance/evidence/m3-offline-latest.json`（执行时 commit `4b34413`；复核时以 HEAD 重跑为准）
- 运行：`bash docs/acceptance/rehearsals/m3-offline.sh` → **exit 0**
- 汇总：**PASS = 23 / FAIL = 0 / BLOCKED = 4 / NOT RUN = 0**
- M2 回归不变式：`bash docs/acceptance/rehearsals/m2-offline.sh` → exit 0（53 PASS / 0 FAIL / 15 BLOCKED，未因 M3 变更退化）

| ID | PASS | FAIL | BLOCKED | NOT RUN | 备注 |
|---|---|---|---|---|---|
| M3-BUILD | 2 | 0 | 0 | 0 | 本机门禁 + 目标提交 CI |
| M3.1 任务恢复 | 4 | 0 | 0 | 0 | 意图日志 + 对账优先级 + 无重复下载 |
| M3.2 备份保留 | 4 | 0 | 0 | 0 | 保留下限 + include 存档 + 备份不影响 stop |
| M3.3 Mod 下载 | 5 | 0 | 1 | 0 | BLOCKED=实机真实下载与 PZ 读取行为观测（需授权） |
| M3.4 就绪查询 | 2 | 0 | 1 | 0 | BLOCKED=声明目标 A2S 可达性实机探测（需授权） |
| M3.5 容量策略 | 4 | 0 | 0 | 0 | 双检 + 恢复路径 + 自激环 + 口径 |
| M3.6 实机长跑 | 2 | 0 | 2 | 0 | BLOCKED=端口占用失败关闭语义复核、8h 观测窗口 |

## 2. 交付与证据索引

| 能力 | 实现 | 证据 |
|---|---|---|
| M3.1 任务恢复（D10 契约偏差） | `internal/application/install_reconcile.go`、`internal/adapters/localstate/intent.go`、`ports.IntentLog` | `TestTaskIntentLogPersistsPhases` / `…ReconcileAfterCrashAvoidsRedownload` / `…ReconcileWithoutContentFailsClosed` / `TestReconcileDefersToRecoveryRequired` |
| M3.2 备份保留与自动备份 | `internal/adapters/migrate/backup.go`（`BackupWithOptions`/`PruneBackups`）、`auto.go`、`retention.go` | `TestPruneNeverRemovesNewestVerifiedBackup` / `TestBackupWithSavesRecordsIncludeMode` / `TestAutomaticBackupFailureDoesNotChangeStopResult` |
| M3.3 Mod 下载（D11 新端点） | `internal/adapters/steamcmd/workshop.go`、`application.DownloadMod`、`httpapi.downloadMod` | `TestLegacyModsEndpointUnchanged` / `TestModDownloadRegistersOnlyAfterSuccess` / `…ConflictsWithInstall` / `TestModContentPathTraversalRejected` |
| M3.4 就绪与查询（D12 追加字段） | `internal/adapters/pz/a2s.go`、`ports.GameQuerier`、`readiness_timeline`/`game_query` 投影 | `TestParseA2SInfo` / `TestQueryFieldsDegradeToUnavailable` / `TestQueryFieldsProjectWhenA2SAnswers` |
| M3.5 容量策略 | `internal/application/capacity.go`、`ports.CapacityChecker`、`CodeCapacityExceeded`→409 | `TestCapacityDoubleCheckRejectsPartialWrite` / `TestCapacityHardLimitAndRecoveryPath` / `TestAutomaticBackupDefersWhenCapacityTight` / `TestDiskUsageExcludesBackupsAndLockDirs` |
| M3.6 实机可靠性 | —（证据见下） | `docs/acceptance/evidence/M3-windows-reliability.md`（10 轮启停 + 4 类注入 + 50 分钟观测窗口） |

## 3. 实机证据摘要（隔离根）

- **连续启停 10/10 轮**（含第 6 轮 `taskkill /F` 强杀后 stop 幂等）：每轮 start=200 / stop=200 / `running:false` / 残留 0 进程 0 端口。
- **崩溃注入**：二进程争锁 → 409 `instance owned by another process`（释放后恢复 200）；缺制品 → start=500 且未起进程；外部改写配置 → 按受管键重写后成功启动。**端口占用注入未证明失败关闭**（start=200 属既有契约「子进程创建成功即 200」）→ 记为未解除项。
- **观测窗口 50 样本 / 约 50 分钟**：`running` 50/50、`readiness=ready` 49/50、`procs` 1/50、关键事件 marker 全程可检索（无静默丢失）；**句柄 4737 → 5340（+603）趋势不足以定论** → 8h 行记 `BLOCKED(时间窗口)`。
- **观测限制（如实记录）**：`/api/status` 在本目标机 `memory_mb`/`cpu_percent` 恒为 0，句柄来自 PowerShell 独立口径。

## 4. 契约与登记

- 本里程碑引入的偏差已在实现前登记：**D10**（安装任务持久化，`LEGACY-CONTRACT.md:193` 语义更新）、**D11**（`POST /api/server/mods/download` 新端点，legacy 登记端点逐字不变）、**D12**（就绪时间线/查询字段/容量字段一律追加，`ready`/`readiness`/`quota_gb`/`usage_percent` 语义不变）。见 ADR §2.3 r5 登记表与 §5.4 偏差表、`LEGACY-CONTRACT.md` §4.2。
- 决策记录：M3.2（保留默认 keep 3、`--include-saves` 实现、`--include-server-files` 明确不实现、自动备份在 sandbox 沉降后且不影响 stop）、M3.5（双检、硬阈值 409、恢复路径、自激环预检、口径）见 `M3-GO-PZ-RELIABILITY.md`。

## 5. 结论与未解除项

**结论**：M3 的**离线可判定行全部 PASS**（23/23 执行行，FAIL=0、NOT RUN=0），实机核心项（连续启停、争锁、缺制品、配置改写、运行期稳定性）在隔离数据根上取得可复核证据。**M3 判定为「离线完成、实机部分完成」**——以下 4 项 BLOCKED 未解除，不得声称 M3 完全通过：

1. **M3.3 实机**：真实 Workshop 下载 + PZ 42.21 读取行为观测（需目标机授权动作）。
2. **M3.4 实机**：声明目标 A2S 可达性一次性 UDP 探测（解析器已实现并单测覆盖）。
3. **M3.6 端口占用**：失败关闭语义未证明（既有契约下 start=200），需游戏日志/就绪复核。
4. **M3.6 观测窗口**：50 分钟不足以判定句柄趋势，需 ≥4h（理想 8h）完整窗口。

**平台声明**：本里程碑证据**不产生** Linux/macOS 支持声明；Windows 声明边界沿用 ADR §1.6 r4/r5（Windows 10 x64 + launcher-descriptor 向量 + PZ 42.21）。
