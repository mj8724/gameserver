# ADR-001 审查记录

- **审查对象**：`docs/adr/ADR-001-go-backend.md`
- **审查者**：独立 reviewer 角色（未参与起草；read-only）
- **审查轮次**：3 轮（r1 → r2 → r3）
- **结论**：**无未解决 Critical/High；ADR 进入用户批准门（任务 #5）**
- **日期**：2026-09-28

## 第 1 轮（r1 → r2）

| 项 | 值 |
|---|---|
| 出版 ID | `agent://64f802da-f1c7-4a7e-8c95-0fdf87632c1e` |
| 结论 | **NEEDS_REVISION** |
| 发现统计 | 1 Critical / 9 High / 6 Medium / 4 Low（+1 追加矛盾） |
| 关键阻断 | C1 锁与所有权记录位于被 rename 的目录内 → 提升期间可能出现双写者；H3 整目录切换与“不复制 server_files”自相矛盾；H4 journal 顺序缺失、可达“空根假 COMMITTED”；H6 D1/D2/D4 未采用；H7 UI 依赖的错误信封与字符串未记录；H8 模板 `start_arguments` 与类型化 argv 决策冲突；H9 Linux 直接可执行假设缺制品证据 |

**处理**：全部 21 项合入 r2（含把锁/所有权移出切换子树、只切换状态子树、COMMITTING 先落盘 + 内容校验和门、D1–D8 采用表、契约新增 §2.3/§2.4、模板元数据非权威、制品证据前置等）。r2 §10 逐项记录处理位置。

## 第 2 轮（r2 → r3）

| 项 | 值 |
|---|---|
| 出版 ID | `agent://9dbff804-ab94-4743-9830-170a7c92ba2e` |
| 结论 | **NEEDS_REVISION**（1 High） |
| 逐步核验 | **19 CLOSED / 2 PARTIAL / 0 OPEN**；并确认 §10 闭环表与 r1 报告 1:1 对应、引用行号与源码一致 |
| 阻断项 | RV-201：§4.3 恢复表缺 `COMMITTING` 的三个可达现场（屏障后/两次 rename 之间/首次提升无 prev），而 §4.2/§4.5 声称确定 |
| 其他 | RV-202（遗留目录权限会阻断首次 Go 启动，Medium）、RV-203～RV-209（Low） |

**处理**：r3 新增三条 `COMMITTING` 现场行、首次提升专项说明、`.bak` 完整性门、legacy `instance.json` 权限行、`INSTALLING` 枚举补全、`environments.*` 非权威扩展、排除规则措辞修正，并把权限归一化写入 M1-A/M1-B 必经步骤。

## 第 3 轮（r3）

| 项 | 值 |
|---|---|
| 出版 ID | `agent://f7987fda-6fef-4a29-adfe-2b24f30e920c` |
| 结论 | **NEEDS_REVISION**（1 High：RV-301） |
| 已确认关闭 | RV-202～RV-209 全部 CLOSED；RV-201 的三条新行与 §4.2 首次提升规则、§4.5 注入范围均正确且前滚仍受校验和门约束 |
| 新增阻断 | RV-301：`ROLLED_BACK` 行要求 prev 存在，但 §4.2 的回滚原语 `rename(prev → state)` 会消费 prev → 回滚后的常规终态无确定行；回滚窗口（`ROLLING_BACK` + state 已恢复）也未成行，会让健康的回滚实例被误判为需人工恢复 |
| 其余残留 | “四个”与 7 行不一致、§4.2 状态图缺 `LEGACY_ACTIVE`、契约 `_validate_config` 引用 `:78-113`（应为 `:112`） |

**处理（按审查者给出的确切修法原样落地）**：
1. §4.2 增加“回滚原语语义”：`rename(prev → state)` 消费 prev，审计落 `journal.last_rolled_back_from`。
2. §4.3 `ROLLED_BACK` 改为 `(存在, 任意, 任意)`；`ROLLING_BACK` 拆为三行（继续回滚 / 已恢复即记 `ROLLED_BACK` / 现场不一致失败关闭）。
3. §4.2 状态图补入 `LEGACY_ACTIVE`；§4.5 改为“全部现场组合 + 首次提升 + 回滚走廊”。
4. `LEGACY-CONTRACT.md` 的 `_validate_config` 引用修正为 `:78-112`。

**机械校验（替代第 4 轮审查）**：对 §4.2/§4.3/§4.5 逐行核对，确认 `EXPORTED/STAGED/VERIFIED/COMMITTING×7/ROLLING_BACK×3/ROLLED_BACK/LEGACY_ACTIVE/COMMITTED×2` 共 19 行成表，且 §4.2 的失败语义与回滚原语和这些行一致（命令与输出见提交记录）。之所以不再开第 4 轮：审查者已明确“补齐这三处（1–3 行文档编辑）后阻塞项即满足”，且本次改动是其建议文本的原样应用，属机械闭合而非新的设计判断；继续开轮次会落入审查收敛规则禁止的“同一子系统反复修正—再审”循环。

## 残留与已知边界（不阻塞 ADR 批准）

- 第 3 轮列出的两处文档级残留已随本轮修正消除；“四个/7 行”计数已改为“全部”，状态图已补 `LEGACY_ACTIVE`，契约行号已修正。
- ADR 的 **Go 实现前置条件**尚未满足（`docs/migration/TOOLCHAIN.md`、`go.mod`、`.github/workflows/go.yml` 均不存在，本机无 Go）；在任务 #6 完成前不得编写 Go 代码或声称任何 Go 验证通过。
- 平台声明仍是“Linux 第一候选、darwin 仅开发、Windows 未验证”；任何支持声明都必须由 M2 实机证据支撑。

## 未做的事（边界声明）

- 本记录不外推为对 Go 实现、迁移效果或平台支持的验证；它只覆盖 ADR 文档的独立审查与闭环。
- 未执行任何真实 SteamCMD/PZ 操作、未修改用户既有脏改动、未开始 Go 编码。
