# M3 验收矩阵：单实例可靠性（M3-GO-PZ-RELIABILITY）

> 依据已批准计划 r15《剩余里程碑计划》§2 M3（批准 2026-09-29）。
> 执行器：`docs/acceptance/rehearsals/m3-offline.sh`（离线行）；实机行由 `TARGET-MANIFEST-windows.md §M3` 授权的目标机动作产生证据。
> 证据口径：全部实机证据标注「**隔离数据根基线验收**」；平台声明仅对 Windows 10 x64（ADR §1.6 r4/r5）成立。

## 1. 判定四态与门禁规则

| 状态 | 含义 | 允许的用法 |
|---|---|---|
| **PASS** | 该行已执行且结论为通过，证据可复核 | 只有实际执行并留证才可记 PASS |
| **FAIL** | 该行已执行且结论为失败 | 必须给出可复现步骤与现场 |
| **BLOCKED** | 未执行且**有明确外部前提缺失**（授权、目标机、时间窗口、具名主机） | 必须写明阻塞条件；不得以 BLOCKED 代替 FAIL |
| **NOT RUN** | 该行所属能力**尚未实现**（M3.0 起点状态） | 能力落地后必须转为 PASS/FAIL；不得长期滞留 |

**门禁规则**：
1. FAIL > 0 → 执行器 exit 1（阻断）。
2. BLOCKED / NOT RUN > 0 → exit 0，但**离线行继续推进，实机行挂起**；签核必须逐 ID 列出四态计数。
3. **不得把 BLOCKED 或 NOT RUN 记作 PASS**，也不得记 N/A（N/A 仅适用于按 r5 平台决策改判的 Linux 行）。
4. 计数必须来自实物证据（`m3-offline-latest.json` 的 commit 字段需等于 HEAD）。

## 2. 矩阵行

| ID | 能力 | 子案例 | 离线判定（命名用例/动作） | 实机判定（Windows 隔离根） |
|---|---|---|---|---|
| M3-BUILD | 门禁 | 固定工具链与本机门禁（gofmt/vet/build/tidy） | 执行器内置 | — |
| M3-BUILD | 门禁 | 目标提交 GitHub CI 全绿（含 race 与 archtest） | CI 查询（三态） | — |
| **M3.1** | 任务恢复 | 意图日志阶段化持久化（REQUESTED→RUNNING→VERIFYING→DONE/FAILED） | `TestTaskIntentLogPersistsPhases` | — |
| **M3.1** | 任务恢复 | 重启对账自洽且无重复下载（build id + 字节 + 指纹三要素） | `TestTaskReconcileAfterCrashAvoidsRedownload` | 安装中途杀进程后重启一次 |
| **M3.1** | 任务恢复 | 对账顺序：陈旧 owner → `RECOVERY_REQUIRED` 优先，不被续跑覆盖 | `TestReconcileDefersToRecoveryRequired` | — |
| **M3.1** | 任务恢复 | 网络失败 → FAILED 且可重试；legacy 四态投影不变 | `TestTaskIntentFailureAndLegacyStatusProjection` | — |
| **M3.1** | 任务恢复 | D10 契约偏差登记后 M2-RESTART 受影响行已更新 | 矩阵文本断言（人工/脚本） | — |
| **M3.2** | 备份保留 | prune 保留 N 且永不删除最后一份可用备份 | `TestPruneBackupsKeepsNewestAndRequiresExplicitCall`（扩展）+ `TestPruneNeverRemovesLastUsableBackup` | — |
| **M3.2** | 备份保留 | 保留策略默认与容量上限（决策记录：默认 keep 3） | 决策记录锚点 + `TestBackupRetentionPolicyDefaults` | — |
| **M3.2** | 自动备份 | PZ 退出且 sandbox 稳定后触发；**备份失败不影响 stop**（stop 200） | `TestAutomaticBackupFailureDoesNotChangeStopResult` | 停止后观察备份产物时间戳晚于 sandbox 重写 |
| **M3.2** | 存档恢复 | 显式 include 存档的备份经 staging+promotion 恢复；世界文件指纹一致 | `TestRestoreWithSavesThroughPromotionKeepsDigest` | 恢复后启动并核对存档可读 |
| **M3.2** | 决策 | `--include-saves`/`--include-server-files` 实行或不实行的显式决策 | 决策记录锚点 | — |
| **M3.3** | Mod 下载 | 前置取证：落盘路径、PZ 读取路径、用量/备份关系、Runner 互斥 | 取证结论落 Manifest §M3（2026-09-30 代码取证 4 项；实机 2 项 BLOCKED：真实下载、PZ 42.21 读取行为） | 一次真实 workshop 下载（授权后）+ 启动含 mod 服务器观察加载日志 |
| **M3.3** | Mod 下载 | legacy `POST /api/server/mods` 逐字不变 | `TestLegacyModsEndpointUnchanged` | — |
| **M3.3** | Mod 下载 | 新端点先下载后登记；失败不写 INI；无效 id 明确错误 | `TestModDownloadRegistersOnlyAfterSuccess` | 真实 mod 下载 → INI → PZ 启动加载（日志含 mod 名） |
| **M3.3** | Mod 下载 | 单在飞互斥（与 install 并发 → 409） | `TestModDownloadConflictsWithInstall` | — |
| **M3.3** | Mod 下载 | 不可信内容边界（不执行/不解析可执行内容/路径穿越白名单） | `TestModContentPathTraversalRejected` | — |
| **M3.4** | 就绪 | A2S 可达性只读取证（声明目标 PZ 42.21） | 取证记录（BLOCKED 或实机） | 一次 UDP A2S_INFO 探测 |
| **M3.4** | 就绪 | 就绪时间线（start→checking→ready/timeout/failed 带时间戳，追加字段） | `TestStatusExposesReadinessTimeline` | 冷/热启动时间线复现（40s/34s 基线，三次中位数 ±10%） |
| **M3.4** | 查询 | 查询字段（玩家数/地图）或 `unavailable` 降级；`ready`/`readiness` 语义不变 | `TestQueryFieldsDegradeToUnavailable` + 既有 readiness 用例全绿 | 查询返回或降级观测 |
| **M3.5** | 容量 | 双检（入口+写盘前）与竞态对抗：采样未超但写入时超 → 拒绝且无部分写入 | `TestCapacityDoubleCheckRejectsPartialWrite` | — |
| **M3.5** | 容量 | 硬阈值 409 + 恢复路径（只读+配置/CLI 回退，永不自动删除） | `TestCapacityHardLimitAndRecoveryPath` | 阈值触发后实测拒绝与恢复 |
| **M3.5** | 容量 | 自激环防护：自动备份前预检自身用量 → `pending_backup` 延迟不失败 | `TestAutomaticBackupDefersWhenCapacityTight` | — |
| **M3.5** | 容量 | 用量口径（实例根递归，排除备份与 `#locks/#owners`）与 `DiskUsageMB` 对齐；阈值禁用=现状 | `TestDiskUsageExcludesBackupsAndLockDirs` | 独立口径误差 <5% |
| **M3.6** | 长跑 | 连续 start/stop ≥10 轮（含 1 次强杀）后进程 0/端口 0/锁无残留 | — | 实机证据（必跑） |
| **M3.6** | 长跑 | 4 类崩溃注入（二进程争锁、端口占用、缺制品、外部改写配置）失败关闭不挂死 | 离线可覆盖部分（争锁/缺制品） | 实机证据（必跑） |
| **M3.6** | 长跑 | 8h 观测（60s 采样；关键事件：`SERVER STARTED`/shutdown 序列/异常退出）无静默丢失 | — | 实机证据；资源不允许时 ≥4h 并标 BLOCKED(时间窗口) |
| **M3.6** | 长跑 | 隧道中断 → 该行 BLOCKED + 现场保留清单与清理责任 | — | 条件触发 |
| **M3.7** | 签核 | 四态计数 + 证据 JSON commit 钉 HEAD + 声明边界引用 + 独立审查注记 | `docs/acceptance/M3-SIGNOFF.md` | — |

## 3. 签核章节模板（固定）

```markdown
## M3 签核（<日期>，commit <sha>）

- 证据：`docs/acceptance/evidence/m3-offline-latest.json`（commit=<sha>，与 HEAD 一致）
- 计数：PASS=<n> / FAIL=<n> / BLOCKED=<n> / NOT RUN=<n>（逐 ID 明细见下）
- 声明边界：Windows 10 x64 + launcher-descriptor 向量 + PZ <build>；Linux/macOS **未验证**（不得声称支持）
- 证据口径：全部实机证据为**隔离数据根基线验收**，不含生产可靠性表述
- 独立审查：**非独立复核**（teammate 额度受限；ADR §11.3 豁免留痕，补做触发=额度恢复）

| ID | PASS | FAIL | BLOCKED | NOT RUN | 备注 |
|---|---|---|---|---|---|
```

## 4. 平台与 N/A 规则

- 按 r5 平台决策：Linux/macOS 相关行若无法在声明目标上评估 → 标 `N/A—平台未声明`（NOT PASS/NOT BLOCKED，证据保留），不计入 FAIL。
- M3 的 Windows 证据**不改变** M2 未验证平台的状态（ADR §1.6）。

## M3.2 决策记录（2026-09-30）

| 决策 | 选择 | 理由 | 证据/代码 |
|---|---|---|---|
| 保留默认 keep | **3**（与 ADR §4.6 一致），容量上限可配 | 与既有默认对齐，prune 永不删除最新一份可用备份 | `internal/adapters/migrate/retention.go` `DefaultBackupKeep` |
| `--include-saves` | **实现**（CLI flag + manifest `includes` 标记 + 自动备份默认包含存档） | 满足"真实存档恢复"验收；自动备份带存档以便恢复演练 | `cmd/gameserver/main.go`、`BackupOptions.IncludeSaves` |
| `--include-server-files` | **明确不实现**（记录决策） | `server_files` 可再生、单次备份 ≥20 GB，会与 M3.5 容量上限互相撕扯；随保留/容量策略工作重新评估 | `BackupOptions` 注释 |
| 自动备份触发 | `stop` 成功后异步、pz 退出 sandbox 重写稳定后（沉降 5s）、`pending_backup`→`last_backup` 状态字段 | **备份失败不影响 stop**（停服契约不变）；备份为尽力而为 | `scheduleAutomaticBackup`、`GAMESERVER_AUTO_BACKUP`/`GAMESERVER_BACKUP_DIR` |

## M3.5 决策记录（2026-09-30）

| 决策 | 选择 | 理由 |
|---|---|---|
| 阈值缺省 | `0` = 禁用 | 保持历史"无限制"行为，策略必须显式开启 |
| 判定点 | API 入口 + 写盘前**双检** | 采样与写入之间可能发生用量跳变（单检会被绕过） |
| 超限动作 | 拒绝新增写入 → 409 `capacity_exceeded` | 只拦增长型写入；只读展示与停止永不受限 |
| 恢复路径 | 调阈值 / 清旧备份（运维动作） | **永不自动删除**；测量失败对增长型写入失败关闭 |
| 自激环 | 自动备份前预检 → `last_backup.state=pending` | 备份自身抬高用量不得导致备份失败或死锁 |
| 口径 | `quota_gb`/`usage_percent` 语义不变；策略用新增 `capacity{}` | 追加字段；用量=实例根递归（不含备份与锁目录） |
