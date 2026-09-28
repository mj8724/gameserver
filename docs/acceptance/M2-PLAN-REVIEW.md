# M2 计划审查记录（**非独立复核**）

> **审查性质：非独立。** 本记录由执行实现的同一主代理完成，用于替代不可用的独立 reviewer。
> 已获用户于 2026-09-28 明确同意此替代方式，并同时确认了计划范围与入口门。
> 独立审查在以下条件满足前应补做：teammate 提供商恢复额度，或用户指定其他审查渠道。

## 为什么没有独立审查

| 尝试 | 结果 |
|---|---|
| 第 1 次独立 reviewer 派发（`agent://1376ff09…`，随后中止） | 读取约 20 分钟后仍无 verdict，捕获输出只有工具调用与推理摘要，**无 findings**；已中止，不能记作审查完成 |
| 第 2 次独立 reviewer 派发（`agent://9cfebb13…`） | 提供商返回 `400 insufficient credits`（模型 `baibbai-copy/commandcode/gpt-6-luna`） |
| 其他 teammate 派发（#8/#9 收尾阶段各一次） | 同样 `400 insufficient credits`；当前 catalog 中所有模型同属该账户 |

用户决定：**接受非独立复核 + 用户确认**。

## 复核范围与方法

对 `docs/acceptance/M2-GO-PZ-MVP.md`（273 行）与 `docs/acceptance/evidence/M2-<os>-<pz-build>.md`（204 行）做了机械与一致性检查：

- **结构**：矩阵 95 行、每行 7 列；14 个 canonical ID 全部出现，计数与文件自报一致（`M2-BUILD`2/`M2-API`3/`M2-UI`2/`M2-AUTH`3/`M2-WS`6/`M2-SINGLEWRITER`13/`M2-SECRET`4/`M2-INSTALL`7/`M2-RESTART`3/`M2-CONFIG`6/`M2-MIGRATE`30/`M2-PROCESS`6/`M2-PZ-LIVE`7/`M2-PLATFORM`3）。
- **可判定性**：每行的 PASS 结果都是可观察断言（状态码、帧类型、进程/PID、校验和、锁状态、时间戳窗口），没有"应当正确"类描述；BLOCKED / NOT RUN / N/A 的使用边界在 §1.3 明确。
- **ADR 一致性**：锁与所有权记录位于被切换子树之外；COMMITTING 七种布局 + 首次提升无 prev + 回滚走廊均成行；前滚要求 state 内容校验和与新 manifest 一致；process-created ≠ ready；60s oracle；`status`/`running`/start 响应不可变、readiness 仅追加；D1（Origin）、D2（logout 吊销 + 重启失效）、D3（error 帧）、D4（deadline/受控 shutdown 取消 + reap，不新增公开路由）；明确排除 M3 持久恢复。
- **门与授权**：正式 M2 结果门在 M1-A switch-ready 之后；live 前置为完整 Target Manifest + 逐项副作用授权；`data/` 与生产根只读；证据文件禁止秘密。
- **上游一致性抽查**：错误 envelope 与 `请先登录` 等字符串对照 `LEGACY-CONTRACT.md` §2.3；`/api/templates` 的 `supported_os` 按兼容数据返回但不作为支持声明。

## 复核发现的偏差与处理

| # | 偏差 | 级别 | 处理 |
|---|---|---|---|
| 1 | ADR 头部仍写 `Proposed r3`，但用户已于决策记录中批准 | 文档级（可能误判门状态） | 已更新 ADR 头部为 **Approved r3** 并引用 `ADR-001-decision.md` |
| 2 | M2 计划 §8 称 `TOOLCHAIN.md` 有"待补说明"，且未记录已取得的 Go CI 绿证 | 文档级（会误读为无 CI 证据） | 已在 `TOOLCHAIN.md` 记录 run 36421008298 / 36436902986 / 36437730487 / 36438544535；计划 §8 的措辞由本记录与决策记录共同修正 |
| 3 | 执行顺序把 M2-BUILD（CI 预检）排在 M1-A 签署之前，未说明它不算正式 M2 PASS | 流程歧义 | 计划 §2 已写明"重叠 M1-A 证据只能在 M1-A 工作项下记录"；本记录补充：M2-BUILD 的 CI 预检属于**前置条件核对**，正式 PASS 仍需在 M1-A 签署后按同一 SHA 重新引用 |
| 4 | `M2-WS` 的 D3 有两行（wire 断言与 UI 稳态） | 可接受 | 两行验证对象不同（原始 WS 客户端 vs 浏览器 UI），不是重复计数；保留 |
| 5 | 平台声明与可用主机：用户现提供一台 **Windows** 测试主机（Cloudflare 隧道） | 需明确 | Windows 仅在具名 runner 上做**条件性**对抗测试，**不产生 Windows 支持声明**；ADR/用户决策中的支持声明目标仍是 Linux Ubuntu LTS。此条已写入决策记录 |

## 结论

- 计划具备执行条件（范围、门、证据与阻断规则自洽），**但本记录不是独立审查**；独立审查仍列为待办（见上）。
- 用户已确认范围与门槛（见 `M2-PLAN-DECISION.md`）。
- 残留风险：非独立复核可能漏掉措辞级分歧；ADR §4.2 "rename 之前崩溃"句与 §4.3 表的已知歧义仍建议后续单独澄清（当前按 §4.3 表与 COMMITTING 屏障执行，不扩大恢复权限）。
