# M2 离线验收证据（E-OFF，darwin 开发平台）

- **提交**：`f0f6df7576e5c26c145989bd527fdf1002e26b24`；**工具链**：`go1.27.1`；**平台**：`darwin/arm64`
- **被测二进制 SHA-256**：`3f8a69db33f9fbb5503b8c896190e6fdfaeeea50e54bd7b037d5f94447f909e5`
- **执行器（可复跑）**：`bash docs/acceptance/rehearsals/m2-offline.sh`；每次运行覆盖本文件与 `m2-offline-latest.json`（逐行证据在 JSON）
- **统计**：PASS 45 / FAIL 0 / BLOCKED 15
- **签核状态**：未签核（见 `docs/acceptance/M2-SIGNOFF.md`）

> 本文件只记录**离线/合成夹具**证据。它不是平台支持声明，也不构成真实 PZ、SteamCMD、
> 目标主机或生产环境的验收；`M2-PZ-LIVE`、`M2-PLATFORM` 及需要具名目标 OS 的行全部 BLOCKED。

## 1. 已执行行

| 矩阵 ID | 用例 | 结果 | 证据 |
|---|---|---|---|
| M2-BUILD | 固定工具链与本机门禁（gofmt/vet/build/tidy） | PASS | go=go1.27.1 commit=f0f6df7576e5c26c145989bd527fdf1002e26b24 binary_sha256=3f8a69db33f9fbb5503b8c896190e6fdfaeeea50e54bd7b037d5f94447f909e5 |
| M2-BUILD | 目标提交 GitHub CI 全绿（含 race 与 archtest） | PASS | run 36498044998 success for f0f6df7576e5c26c145989bd527fdf1002e26b24 https://github.com/mj8724/gameserver/actions/runs/36498044998 |
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

## 3. 判定说明

- PASS 判定来自单次 `go test -json ./...` 的具名用例结果（`E-OFF`）、目标提交 GitHub CI 结果（run ID + SHA），
  以及真实装配集成测试 `cmd/gameserver`（真实 HTTP 传输、state store、PZ INI 适配器、模板与锁；仅出站游戏适配器可注入 fake）。
- 端到端行（install → is_installed → start → command → stop）验证装配与状态机，**不证明**真实 PZ 能启动。
- 源码级断言（`TestStaticUIContract`）只证明 UI 与路由/文案/帧过滤的一致性，**不替代**浏览器闭环。
- Windows 进程树终止目前只验证 argv 形状（类型化、无 shell）；真实运行仍需 Windows 目标机。
- 跨进程单写者栅栏在 `docs/migration/rehearsals/m1a-rehearsal.sh` 中以**独立 OS 进程**（python `flock` 持锁）验证 409。
- 需要 Linux/Windows 目标主机、浏览器或真实 PZ 的行未执行，不得记为 PASS 或 N/A。
- CI 门为三态：success→PASS、明确失败→FAIL、运行中/无运行→BLOCKED（不误判为失败，脚本可重复执行）。
