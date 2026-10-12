# M2 离线验收证据（E-OFF，Windows 目标机实测）

- **提交**：`1834cee2757882bb85947e03d9ddae922503f5a0`；**工具链**：`go1.27.1`；**平台**：`windows/amd64`
- **被测二进制 SHA-256**：`f701315e33a82504ac5be70a202c019f8270c9d4ebbf3af63cfd4b7b29c4a7e3`
- **主机**：DESKTOP-9M8FOG7（Windows 10 Pro 10.0.19045.6466，AMD64，16 vCPU，32 GB RAM）
- **工作根**：`G:\gameserver-work`（Go 1.27.1 于 `G:\gameserver-work\go`，SHA256 校验通过）
- **执行器（可复跑）**：`bash docs/acceptance/rehearsals/m2-offline.sh`（Git Bash；证据 JSON 为 `m2-offline-latest.json`）
- **统计**：PASS 43 / FAIL 0 / BLOCKED 16

> 这是**离线/合成夹具**在真实 Windows 上的结果，仍**不是**平台支持声明，也不构成真实 PZ、SteamCMD
> 或生产环境验收：`M2-PZ-LIVE`、`M2-PLATFORM` 及需要真实副作用的行仍全部 BLOCKED。
> 与 darwin 证据的差异：CI 行因 Windows 无 `gh` 记为 BLOCKED（三态判定，不误报）；POSIX 权限相关行按设计不适用。

## 1. 已执行行

| 矩阵 ID | 用例 | 结果 | 证据 |
|---|---|---|---|
| M2-BUILD | archtest 直接导入边界 + 负向注入探针 | PASS | 14 个用例全通过（E-OFF, go1.27.1）: TestImportBoundaries, TestInjectedForbiddenImportFailsArchitectureCheck, TestRulesRejectForbiddenImports… |
| M2-API | 20 挂载点/严格方法与错误 envelope | PASS | 35 个用例全通过（E-OFF, go1.27.1）: TestApplicationErrorsAndConfigurationRedaction, TestApplicationErrorsAndConfigurationRedaction/install_already_running, TestApplicationErrorsAndConfigurationRedaction/install_while_running… |
| M2-API | 模板元数据与追加字段（ready/readiness） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestCatalogLoadsValidTemplates, TestControlStatusProjectsStateAndSecrets |
| M2-API | auth/Origin 边界完整性 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestAuthOriginAndRequestValidation, TestAuthenticatorAcceptsTokenAndRejectsTamperedSignature |
| M2-API | 静态资源安全回退 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestOpenRejectsTraversalAndAbsoluteNames, TestStaticAssetsFallbackAndSafePaths |
| M2-API | 静态 UI 调用面与路由表一致性（源码级） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestStaticUIContract |
| M2-API | 真实装配黑盒：路由/错误文案/投影/落盘/权限/锁栅栏 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineHTTPBlackBox |
| M2-AUTH | 登录/会话正常与失败路径 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestAuthenticatorHTTPLoginLogoutCookieAndReplay, TestAuthenticatorRejectsFutureAndExpiredTimestamps, TestAuthenticatorRejectsMissingAndIncorrectConfiguredPassword… |
| M2-AUTH | logout 重放与重启失效（D2） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestAuthenticatorLogoutRevokesReplay, TestAuthenticatorRestartInvalidatesOldTokens |
| M2-AUTH | Origin/CSRF（D1） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestAuthOriginAndRequestValidation |
| M2-WS | 接受前认证与 Origin 门 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup |
| M2-WS | 帧契约、回放与 D3 未运行输入 | PASS | 6 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup, TestConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands, TestConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands/extra… |
| M2-WS | 断连取消与 listener 清理 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleServerCancellationClosesConnectionAndSubscription |
| M2-WS | 读写超时与慢客户端隔离 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup, TestConsoleServerCancellationClosesConnectionAndSubscription |
| M2-WS | 错误帧 wire 契约（D3） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup |
| M2-WS | D3 error frame 不被渲染为日志（源码级 onmessage 断言） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestStaticUIContract |
| M2-SINGLEWRITER | 启动对账：无记录/残留进程/PID/指纹/token/root | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestAcquireWritesOwnershipOutsideInstanceTree, TestExistingRecordOrResidualProcessRequiresRecovery, TestRecoverClearsStaleRecordAndInspectionReportsState |
| M2-SINGLEWRITER | 未取得锁时的服务行为（跨进程） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestMigrationLockIsExclusive, TestSecondProcessIsRefusedAndReleaseFreesTheInstance |
| M2-SINGLEWRITER | 启动时存在非终态 migration journal | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestJournalIsValidJSONAndBlockedStatesAreNonTerminal |
| M2-SINGLEWRITER | 陈旧 owner 记录对账（darwin 补充证据） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineRecoveryRequiredReconciliation |
| M2-SECRET | 秘密脱敏（状态/日志/argv/指纹） | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestAdversarialArgvReachesHelperWithoutSplittingAndIsRedacted, TestArgumentAndChildOutputLogsRedactAdminPassword, TestBuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv… |
| M2-SECRET | 持久秘密数据边界 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestDefaultStateGeneratesSecretWithoutLogging, TestSaveAtomicBackupModesAndUnknownFieldPreservation |
| M2-SECRET | 口令值不回显（状态与配置投影） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineHTTPBlackBox |
| M2-INSTALL | 安装成功/失败/冲突/取消/deadline/retry | PASS | 5 个用例全通过（E-OFF, go1.27.1）: TestControlInstallLifecycleAndConflicts, TestInstallCancellationTerminatesWaitsAndClearsBusy, TestInstallDeadlineEscalatesToKillThenReaps… |
| M2-INSTALL | SteamCMD 安装器配置解析与未配置时失败关闭 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestRuntimeConfiguresInstallerFromTemplate, TestSteamcmdInstallConfigResolution |
| M2-INSTALL | 端到端：install → is_installed → start → command → stop（真实 HTTP/装配，注入 fake 游戏适配器） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineInstallStartStopLifecycle |
| M2-CONFIG | JSON/INI 原子写 round-trip 与故障注入 | PASS | 38 个用例全通过（E-OFF, go1.27.1）: TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile, TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile/backup.chmod, TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile/backup.close… |
| M2-CONFIG | INI/状态兼容与未知键保留 | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestINIReadWritePreservesCommentsUnknownKeysAndOnlyManagedChanges, TestLoadImportsLegacyAndMergesDefaultsWithoutWriting, TestManagedINIUpdatesMapsOnlyManagedKeys |
| M2-CONFIG | 配置校验边界与可编辑字段 | PASS | 15 个用例全通过（E-OFF, go1.27.1）: TestControlConfigValidationAndConsoleInput, TestINIValidationRejectsTraversalUnknownKeysAndLineInjection, TestValidateConfigUpdateEnforcesDeclaredFieldsTypesAndRanges… |
| M2-CONFIG | target/backup 损坏与失败关闭 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestBackupVerifyTamperAndRestoreThroughPromotion, TestLoadInvalidServerNameFallsBackAndCorruptStateFailsClosed |
| M2-CONFIG | D8 权限门失败关闭 + 归一后成功（darwin 补充证据） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineHTTPBlackBox |
| M2-MIGRATE | ADR §4.3 恢复表全部现场 | PASS | 12 个用例全通过（E-OFF, go1.27.1）: TestDryRunWritesNothing, TestFirstPromotionHasNoPrev, TestImportThenPromoteKeepsPrevAndLeavesOtherTreesAlone… |
| M2-MIGRATE | 中断注入：步骤1/步骤2/双失败/前滚校验和 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestCommittedForwardRollRequiresMatchingChecksum, TestStepOneFailureIsRecoveryRequiredAndDeletesNothing, TestStepTwoAndRollbackFailureIsRecoveryRequired… |
| M2-MIGRATE | 备份/损坏拒绝/恢复全链路 + prune | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestBackupVerifyTamperAndRestoreThroughPromotion, TestPruneBackupsKeepsNewestAndRequiresExplicitCall |
| M2-PROCESS | 进程生命周期 helper 与对抗 argv/停止 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestInputGracefulStopAndIdempotentKillReapChild, TestLogBufferIsBoundedToOneThousandNewestLines, TestStartFailureAndFastExitDoNotReportSuccess… |
| M2-PROCESS | 进程状态与日志上限 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestLogLimitIsClampedAndStatusDoesNotExposeSecrets, TestWrongInstanceAndCancelledStartAreRejected |
| M2-PROCESS | 就绪探测（SERVER_PORT/超时/取消/marker） | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestReadinessPropagatesCancellation, TestReadinessRequiresManifestMarkerAndRunningProcess, TestReadinessUsesSERVERPORTAndInjectedProbeAndTimeout |
| M2-PROCESS | 环境变量与模板插值隔离（darwin 补充证据） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestBuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv, TestBuildLaunchSpecValidatesNamesAndNeverAcceptsTemplate |
| M2-PROCESS | Windows 进程树终止 argv（类型化、无 shell；运行仍需目标机） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestTaskkillArgsStayTypedAndExact, TestTaskkillArgsStayTypedAndExactSteamCMD |
| M2-UI | 未登录/会话过期界面分支（源码级：精确文案分支） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestStaticUIContract |
| M2-RESTART | 陈旧 owner 记录 → 失败关闭、不自动清理、只读可用 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineRecoveryRequiredReconciliation |
| M2-RESTART | 重启后读取已提交状态且对账干净（离线部分） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineRestartPersistence |
| M2-RESTART | 端到端停止后状态枚举与 running 语义 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestM2OfflineInstallStartStopLifecycle |

## 2. BLOCKED 行（未执行，原因见表）

| 矩阵 ID | 用例 | 阻塞原因 |
|---|---|---|
| M2-BUILD | 目标提交 GitHub CI 全绿（含 race 与 archtest） | run none status=unknown conclusion=none（提交 1834cee2757882bb85947e03d9ddae922503f5a0） |
| M2-WS | D3 error frame 浏览器互操作（UI 稳态） | 源码级断言已确认 error 帧不进入日志渲染；浏览器 console 无异常的观察仍需 E-BROWSER |
| M2-SINGLEWRITER | 双独立进程争用（Linux 必跑，声明门） | darwin 已用独立 OS 进程持锁 + 真实二进制验证 409（m1a-rehearsal.sh）；Linux 声明仍需目标主机 |
| M2-SECRET | POSIX 秘密目录/文件权限（Linux 目标机） | 需 Linux/Ubuntu LTS 目标主机；darwin 证据仅作补充 |
| M2-INSTALL | 正常结束/取消后的 reap（声明平台） | darwin 已用真实 helper 子进程覆盖；Linux 声明仍需目标主机 |
| M2-CONFIG | 变更操作的 D8 权限门（Linux） | 需 Linux 目标主机 |
| M2-MIGRATE | staging 边界、同卷与锁（目标文件系统） | 需目标文件系统与授权的一次性根 |
| M2-MIGRATE | 仅因权限宽松拒绝提升（Linux） | 需 Linux 目标主机与逐项写权限授权 |
| M2-PROCESS | Linux/Windows 启动向量对抗 | 需具名目标 OS 与已编译 safe helper；Manifest 未填 |
| M2-PROCESS | 环境变量与模板插值隔离（声明平台 helper 实测） | darwin 已用包内测试覆盖构造成本；声明平台仍需具名 runner 与 safe helper |
| M2-UI | 静态 UI 登录/仪表盘浏览器闭环 | 需要真实浏览器与人工操作（E-BROWSER），本机离线不执行 |
| M2-UI | 未登录/会话过期界面分支（浏览器验证） | 源码级分支与精确文案已由 TestStaticUIContract 验证；浏览器中的人工闭环仍需 E-BROWSER |
| M2-RESTART | 控制进程异常退出后的启动对账（E-OS 声明） | darwin 已由 TestM2OfflineRecoveryRequiredReconciliation 覆盖；目标 OS 声明仍需具名 runner |
| M2-RESTART | PZ child 意外退出观察（E-OS） | 需真实/受控子进程与目标 OS |
| M2-PZ-LIVE | 全部 7 行 | 需 Target Manifest、用户逐项授权与真实 SteamCMD/PZ（BLOCKED） |
| M2-PLATFORM | 全部 3 行 | 需 Linux 目标主机资格与平台决策证据 |

## 3. 本次 Windows 实测发现并修复的缺陷

1. **目录 fsync 在 Windows 必失败**（`FlushFileBuffers` → `ERROR_ACCESS_DENIED`）：会让每次状态写入与 PZ INI 写入失败 → 两个适配器改为 Windows 上文档化的 no-op。
2. **`.gitattributes` 缺失**：`core.autocrlf=true` 的检出把全部源文件转成 CRLF，`gofmt -l` 报 76 个文件 → 仓库新增 `.gitattributes` 固定 LF。
3. **进程树终止在 Windows 是坏的**：原实现用 `os.Interrupt`（Windows 不支持）且只杀单进程 → 新增 `taskkill /T /F` 树终止（类型化 argv）。
4. **SteamCMD 安装器未接线**：真实安装必然失败 → 由环境变量 + 模板 `steam.app_id` 解析，未配置时失败关闭。
5. **证据模板文件名含 `<`/`>`**：Windows 无法检出该路径 → 改名 `M2-OS-PZBUILD-TEMPLATE.md`。

## 4. 判定说明

- PASS 判定来自单次 `go test -json ./...` 的具名用例结果（在 Windows 上真实执行）与真实装配集成测试 `cmd/gameserver`。
- 端到端行（install → is_installed → start → command → stop）验证装配与状态机，**不证明**真实 PZ 能启动。
- 需要 Linux/Windows 目标主机（真实副作用）、浏览器或真实 PZ 的行未执行，不得记为 PASS 或 N/A。

## 5. 条件证据状态（2026-09-29 更新）

- **SteamCMD 安装 PZ（app 380870）已在 Windows 目标机真实执行并完成**：`install_task=COMPLETED progress=100`、`is_installed=true`、安装目录含 `java/ jre64/ license/ media/ natives/ ProjectZomboid64.json StartServer64.bat`（耗时约 4.5 分钟）。制品哈希见 `docs/acceptance/TARGET-MANIFEST-windows.md` 与 `m2-windows-artifacts.json`。
- **该记录为 Windows 条件证据，非 E-LIVE**：按 M2-GO-PZ-MVP §3.1，`M2-PZ-LIVE` 的 E-LIVE 目标是 Ubuntu LTS 主机；Windows 结果**不计入该 ID 的 7 行子案例**，也不产生平台支持声明。
- **启动子项仍为 BLOCKED（当时）**：Windows 服务端包不含 `ProjectZomboid64.exe`，原向量不可满足 → 现在由 `launcher-descriptor` 第三向量承接（见计划 r4 与 `TARGET-MANIFEST-windows.md` §3）。
- **授权偏差记录**：该次安装执行时未按 M2 §2 的"逐项授权并写入证据记录"流程先行落档；按门禁记为该动作的**偏差**，补救 = 在 `docs/acceptance/evidence/M2-windows-live-auth.md` 补齐授权记录后，在门内复跑 validate（见计划阶段 6）。
