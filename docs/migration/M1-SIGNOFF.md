# M1-A 签核记录（切换就绪，非接管）

- **日期**：2026-09-28
- **受测提交**：`0aabc78`（Go CI run 36444780723 全绿）+ 本次新增 `export-back` / `fix-permissions` / `restore` CLI 的同分支后续提交
- **环境**：macOS（darwin/arm64，开发平台），隔离副本数据根（`mktemp` 临时目录），不触碰仓库 `data/`、不下载 SteamCMD/PZ、不监听对外端口（仅 `127.0.0.1`）
- **脚本（可复现）**：`docs/migration/rehearsals/m1a-rehearsal.sh`、`docs/migration/rehearsals/smoke-baseline.sh`

## 1. 演练结果（本机 darwin，隔离副本）

| # | 检查项 | 结果 | 证据 |
|---|---|---|---|
| 1 | 真实二进制启动 | PASS | `gameserver ... listening on http://127.0.0.1:18801` |
| 2 | 管理员登录（会话 Cookie） | PASS | `{"authenticated":true}` |
| 3 | ADR §1.7 权限归一（legacy 根 0755 → 0700/0600） | PASS | `fix-permissions` 输出 + PZ INI 写入由 500 转为 200 |
| 4 | 配置读写 + Go 状态持久化 | PASS | `state/instance.json`（0600）含 `"MAX_PLAYERS": 24` |
| 5 | INI 保留未知键/注释 | PASS | `UnknownKey=keep-me` 仍在 INI 中 |
| 6 | **跨进程单写者栅栏** | PASS | 另一 OS 进程持有 `.locks/pz_01.lock` 时，变更请求返回 **409 `instance owned by another process`** |
| 7 | 备份创建（state + Server/*.ini） | PASS | `backup:` 输出 manifest 与 checksum |
| 8 | 备份篡改一字节被拒 | PASS | `restore` 返回 `manifest checksum mismatch`，不写入 |
| 9 | 备份恢复 → journaled promotion | PASS | `restore: promote action=committed` |
| 10 | `export-back` → Python 兼容 `instance.json` | PASS | legacy 文件出现 Go 写入值 |
| 11 | promotion 中断恢复矩阵（包内故障注入） | PASS | `TestRecoveryTableRows`、`TestStepOneFailure…`、`TestStepTwo…`、`TestCommittedForwardRoll…`、`TestBackup…` 全通过 |
| 12 | 优雅关闭 | PASS | SIGTERM → `exit=0`，日志 `shutdown requested; draining` |
| 13 | 本机 Go 门禁（gofmt/vet/test/race/build/tidy） | PASS | 全通过；`GOOS=linux amd64`、`GOOS=windows amd64` 交叉构建通过 |
| 14 | 目标提交 GitHub Go CI | PASS | run 36444780723（含 race 与 architecture boundaries） |

## 2. 未完成 / BLOCKED 项（不得视为 M1 完成）

| 项 | 状态 | 阻塞原因 | 解除条件 |
|---|---|---|---|
| 独立 reviewer 审查（M1-A 证据与 diff） | **BLOCKED** | teammate 提供商额度耗尽（`400 insufficient credits`）；用户已同意以“非独立复核”替代 M2 计划审查，但 M1 签核的独立审查仍建议补做 | 恢复额度或指定替代审查渠道 |
| Windows 目标机实机验收（oslock `LockFileEx`、process Windows 分支、PZ Windows） | **BLOCKED** | Cloudflare Tunnel 无在线 connector（`curl` 返回 530/1033），本机亦无 Access 令牌；用户正在修 | 隧道 connector 恢复 + Access 客户端认证 + `admin` 密钥登录 |
| Linux（Ubuntu LTS）实机验收 | **BLOCKED** | 无具名主机 | 用户提供主机 |
| 双独立 OS 进程 × 每个声明平台 | **PARTIAL** | 本机 darwin 已用独立 OS 进程（python flock 持锁）验证栅栏；Windows/Linux 未验证 | 相应平台可用后在目标机重跑 |
| M1-B 生产接管 | **未授权** | 需具名主机 + 维护窗口 + 用户单独授权 | 用户明确授权 |

## 3. 结论

- **Go 基线在本机隔离副本上达到“切换就绪（M1-A）”的可验证状态**：迁移工具、单写者栅栏、备份/恢复、promotion 中断恢复、export-back 对账、优雅关闭与 CI 均有证据。
- **M1 不得宣布完成**：目标平台（Windows 优先、Linux 次之）实机验收与独立审查仍未完成；未执行任何生产接管。
- 未验证任何平台支持声明；Windows/Linux 在实机证据出现前保持“未验证”。

## 4. 复现记录（2026-09-29 重新核验）

- 已批准的三个运维命令（`restore`、`export-back`、`fix-permissions`）曾在早期被演练脚本中的一条 `git checkout -- .` 误删（演练脚本不得修改仓库）；已恢复并通过重新演练：`14/14 PASS`，且演练/验收脚本现已不再触碰 git 状态。
- 期间发现并修复两个真实缺陷：① HTTP 错误映射把 `OPERATION_FAILED` 吞成 `服务器内部错误`，掩盖了真实原因，现按 legacy 语义回显具体文案；② PZ INI 写入不认识 `json.Number`（HTTP 解码形态），导致 `MAX_PLAYERS` 回退默认值而**未真正更新 INI**，现两者均有回归测试。
- 上述修复后：M1-A 演练 14/14 PASS；`docs/acceptance/evidence/M2-darwin-pre-live.md` 记录的 M2 离线矩阵 30 PASS / 0 FAIL / 16 BLOCKED。

## 5. 条件关闭注记（r5，2026-09-29）

按 ADR §11.3（r5）：**M1-B 退役**（Python 原型不回生产、不做维护窗口切换与 Python 回滚演练），M1 以「**条件关闭**」处理——M1-A 14/14 + M2 Windows 声明达成即关闭。本节取代 §2/§3 中「M1 不得宣布完成」的原措辞（该措辞在 M1-B 仍在范围内时成立）。

**条件关闭的前置与豁免**：
- 独立 reviewer 签署门**豁免**（豁免人=用户 2026-09-29 计划批准；补做触发=teammate 额度恢复或指定替代渠道后对 M1-A + M2 证据补审）。
- Windows 实机项已由 M2 的实机证据承接（`docs/acceptance/evidence/M2-windows-live.md`），不再以「隧道无 connector」为阻塞理由。
- Linux 实机项按 r5 平台决策改判 `N/A—平台未声明`（NOT PASS/NOT BLOCKED，证据保留可追溯）。
