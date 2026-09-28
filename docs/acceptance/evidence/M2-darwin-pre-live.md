# M2 离线验收证据（E-OFF，darwin 开发平台）

- **提交**：`dbb7c57f2a33da8f8c4dda90d9204e3b63d92e0e`；**工具链**：`go1.27.1`；**平台**：`darwin/arm64`
- **被测二进制 SHA-256**：`b607c2a7ef713e34850ccb18f1371163e5c6c0e164c78e80d7d595707df8ddaf`
- **执行器（可复跑）**：`bash docs/acceptance/rehearsals/m2-offline.sh`（输出本文件的 JSON 版本）
- **统计**：PASS 30 / FAIL 0 / BLOCKED 16

> 本文件只记录**离线/合成夹具**证据。它不是平台支持声明，也不构成真实 PZ、SteamCMD、
> 目标主机或生产环境的验收；`M2-PZ-LIVE`、`M2-PLATFORM` 及需要具名目标 OS 的行全部 BLOCKED。

## 1. 已执行行

| 矩阵 ID | 用例 | 结果 | 证据 |
|---|---|---|---|
| M2-BUILD | 固定工具链与本机门禁（gofmt/vet/build/tidy） | PASS | go=go1.27.1 commit=dbb7c57f2a33da8f8c4dda90d9204e3b63d92e0e binary_sha256=b607c2a7ef713e34850ccb18f1371163e5c6c0e164c78e80d7d595707df8ddaf |
| M2-BUILD | archtest 直接导入边界 + 负向注入探针 | PASS | 14 个用例全通过（E-OFF, go1.27.1）: TestImportBoundaries, TestInjectedForbiddenImportFailsArchitectureCheck, TestRulesRejectForbiddenImports… |
| M2-API | 20 挂载点/严格方法与错误 envelope | PASS | 35 个用例全通过（E-OFF, go1.27.1）: TestApplicationErrorsAndConfigurationRedaction, TestApplicationErrorsAndConfigurationRedaction/install_already_running, TestApplicationErrorsAndConfigurationRedaction/install_while_running… |
| M2-API | 模板元数据与追加字段（ready/readiness） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestCatalogLoadsValidTemplates, TestControlStatusProjectsStateAndSecrets |
| M2-API | auth/Origin 边界完整性 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestAuthOriginAndRequestValidation, TestAuthenticatorAcceptsTokenAndRejectsTamperedSignature |
| M2-API | 静态资源安全回退 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestOpenRejectsTraversalAndAbsoluteNames, TestStaticAssetsFallbackAndSafePaths |
| M2-AUTH | 登录/会话正常与失败路径 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestAuthenticatorHTTPLoginLogoutCookieAndReplay, TestAuthenticatorRejectsFutureAndExpiredTimestamps, TestAuthenticatorRejectsMissingAndIncorrectConfiguredPassword… |
| M2-AUTH | logout 重放与重启失效（D2） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestAuthenticatorLogoutRevokesReplay, TestAuthenticatorRestartInvalidatesOldTokens |
| M2-AUTH | Origin/CSRF（D1） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestAuthOriginAndRequestValidation |
| M2-WS | 接受前认证与 Origin 门 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup |
| M2-WS | 帧契约、回放与 D3 未运行输入 | PASS | 6 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup, TestConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands, TestConsoleMalformedUnknownAndArbitraryTextNeverBecomeCommands/extra… |
| M2-WS | 断连取消与 listener 清理 | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleServerCancellationClosesConnectionAndSubscription |
| M2-WS | 读写超时与慢客户端隔离 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup, TestConsoleServerCancellationClosesConnectionAndSubscription |
| M2-WS | 错误帧 wire 契约（D3） | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestConsoleAuthOriginReplayPingInputStoppedFrameAndCleanup |
| M2-SINGLEWRITER | 启动对账：无记录/残留进程/PID/指纹/token/root | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestAcquireWritesOwnershipOutsideInstanceTree, TestExistingRecordOrResidualProcessRequiresRecovery, TestRecoverClearsStaleRecordAndInspectionReportsState |
| M2-SINGLEWRITER | 未取得锁时的服务行为（跨进程） | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestMigrationLockIsExclusive, TestSecondProcessIsRefusedAndReleaseFreesTheInstance |
| M2-SINGLEWRITER | 启动时存在非终态 migration journal | PASS | 1 个用例全通过（E-OFF, go1.27.1）: TestJournalIsValidJSONAndBlockedStatesAreNonTerminal |
| M2-SECRET | 秘密脱敏（状态/日志/argv/指纹） | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestAdversarialArgvReachesHelperWithoutSplittingAndIsRedacted, TestArgumentAndChildOutputLogsRedactAdminPassword, TestBuildLaunchSpecDoesNotUseTemplateAndPreservesAdversarialArgv… |
| M2-SECRET | 持久秘密数据边界 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestDefaultStateGeneratesSecretWithoutLogging, TestSaveAtomicBackupModesAndUnknownFieldPreservation |
| M2-INSTALL | 安装成功/失败/冲突/取消/deadline/retry | PASS | 5 个用例全通过（E-OFF, go1.27.1）: TestControlInstallLifecycleAndConflicts, TestInstallCancellationTerminatesWaitsAndClearsBusy, TestInstallDeadlineEscalatesToKillThenReaps… |
| M2-CONFIG | JSON/INI 原子写 round-trip 与故障注入 | PASS | 38 个用例全通过（E-OFF, go1.27.1）: TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile, TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile/backup.chmod, TestAtomicWriteFaultsNeverReturnSuccessAndLeaveWholeOldOrNewFile/backup.close… |
| M2-CONFIG | INI/状态兼容与未知键保留 | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestINIReadWritePreservesCommentsUnknownKeysAndOnlyManagedChanges, TestLoadImportsLegacyAndMergesDefaultsWithoutWriting, TestManagedINIUpdatesMapsOnlyManagedKeys |
| M2-CONFIG | 配置校验边界与可编辑字段 | PASS | 15 个用例全通过（E-OFF, go1.27.1）: TestControlConfigValidationAndConsoleInput, TestINIValidationRejectsTraversalUnknownKeysAndLineInjection, TestValidateConfigUpdateEnforcesDeclaredFieldsTypesAndRanges… |
| M2-CONFIG | target/backup 损坏与失败关闭 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestBackupVerifyTamperAndRestoreThroughPromotion, TestLoadInvalidServerNameFallsBackAndCorruptStateFailsClosed |
| M2-MIGRATE | ADR §4.3 恢复表全部现场 | PASS | 12 个用例全通过（E-OFF, go1.27.1）: TestDryRunWritesNothing, TestFirstPromotionHasNoPrev, TestImportThenPromoteKeepsPrevAndLeavesOtherTreesAlone… |
| M2-MIGRATE | 中断注入：步骤1/步骤2/双失败/前滚校验和 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestCommittedForwardRollRequiresMatchingChecksum, TestStepOneFailureIsRecoveryRequiredAndDeletesNothing, TestStepTwoAndRollbackFailureIsRecoveryRequired… |
| M2-MIGRATE | 备份/损坏拒绝/恢复全链路 + prune | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestBackupVerifyTamperAndRestoreThroughPromotion, TestPruneBackupsKeepsNewestAndRequiresExplicitCall |
| M2-PROCESS | 进程生命周期 helper 与对抗 argv/停止 | PASS | 4 个用例全通过（E-OFF, go1.27.1）: TestInputGracefulStopAndIdempotentKillReapChild, TestLogBufferIsBoundedToOneThousandNewestLines, TestStartFailureAndFastExitDoNotReportSuccess… |
| M2-PROCESS | 进程状态与日志上限 | PASS | 2 个用例全通过（E-OFF, go1.27.1）: TestLogLimitIsClampedAndStatusDoesNotExposeSecrets, TestWrongInstanceAndCancelledStartAreRejected |
| M2-PROCESS | 就绪探测（SERVER_PORT/超时/取消/marker） | PASS | 3 个用例全通过（E-OFF, go1.27.1）: TestReadinessPropagatesCancellation, TestReadinessRequiresManifestMarkerAndRunningProcess, TestReadinessUsesSERVERPORTAndInjectedProbeAndTimeout |

## 2. BLOCKED 行（未执行，原因见表）

| 矩阵 ID | 用例 | 阻塞原因 |
|---|---|---|
| M2-BUILD | 目标提交的 GitHub CI 全绿（race/archtest） | 需 GitHub Actions 结果，见 docs/migration/TOOLCHAIN.md 记录（本脚本不联网） |
| M2-WS | D3 error frame 浏览器互操作 | 需要浏览器与 UI 侧观察（E-BROWSER） |
| M2-SINGLEWRITER | 双独立进程争用（Linux 必跑） | darwin 已补充验证，但矩阵要求 Linux；目标主机缺失 |
| M2-SECRET | POSIX 秘密目录/文件权限（Linux 目标机） | 需 Linux/Ubuntu LTS 目标主机；darwin 证据仅作补充 |
| M2-INSTALL | 正常结束/取消后的 reap（真实 helper，Linux） | 需 Linux 目标主机 |
| M2-CONFIG | 变更操作的 D8 权限门（Linux） | 需 Linux 目标主机 |
| M2-MIGRATE | staging 边界、同卷与锁（目标文件系统） | 需目标文件系统与授权的一次性根 |
| M2-MIGRATE | 仅因权限宽松拒绝提升（Linux） | 需 Linux 目标主机与逐项写权限授权 |
| M2-PROCESS | Linux/Windows 启动向量对抗 | 需具名目标 OS 与已编译 safe helper；Manifest 未填 |
| M2-PROCESS | 环境变量与模板插值隔离（声明平台） | 需声明平台 runner |
| M2-UI | 静态 UI 登录/仪表盘浏览器闭环 | 需要真实浏览器与人工操作（E-BROWSER），本机离线不执行 |
| M2-UI | 未登录/会话过期界面分支 | 同上；服务端 401 detail 已由 httpapi 测试与黑盒覆盖，UI 分支未在浏览器中验证 |
| M2-RESTART | 控制进程异常退出后的启动对账（E-OS） | 需目标 OS 与隔离 helper child |
| M2-RESTART | PZ child 意外退出观察（E-OS） | 需真实/受控子进程与目标 OS |
| M2-PZ-LIVE | 全部 7 行 | 需 Target Manifest、用户逐项授权与真实 SteamCMD/PZ（BLOCKED） |
| M2-PLATFORM | 全部 3 行 | 需 Linux 目标主机资格与平台决策证据 |

## 3. 判定说明

- PASS 判定来自单次 `go test -json ./...` 的具名用例结果（`E-OFF`），以及真实装配集成测试 `cmd/gameserver`：
  真实 state store、PZ INI 适配器、模板目录与真实 handler，仅 listener 由 `httptest` 注入。
- 跨进程单写者栅栏在 `docs/migration/rehearsals/m1a-rehearsal.sh` 中以**独立 OS 进程**（python `flock` 持锁）验证 409；
  本文件中的锁判定使用同进程锁持有者，属补充证据。
- 需要 Linux/Windows 目标主机、浏览器或真实 PZ 的行未执行，不得记为 PASS 或 N/A。
