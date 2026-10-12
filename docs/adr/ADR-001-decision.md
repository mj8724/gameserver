# ADR-001 用户决策记录

- **时间**：2026-09-28（本地）
- **决策方式**：`ask-user-question` 结构化工单（4 问）
- **依据**：已批准 Plan `86c3e976a061fc28b352f996708826a95c395a6600595d2b4cf9f13f1b8fc8f0`「M1/M2 完成定义」与「审查与控制门 6」

## 用户答复（原样记录）

| # | 问题 | 用户选择 |
|---|---|---|
| 1 | 是否批准 `docs/adr/ADR-001-go-backend.md`（r3）作为 M1/M2 实现依据？ | **批准 ADR-001 r3** |
| 2 | 这次执行到哪一步为止？ | **要求 M1-B 实际接管** |
| 3 | 本机无 Go 工具链，是否允许安装？ | **允许官方压缩包安装到项目外**（不使用系统包管理器） |
| 4 | M2 目标平台声明范围？ | **仅 Linux（Ubuntu LTS）** |

## 由此生效的约束变更

1. **ADR 状态**：`docs/adr/ADR-001-go-backend.md` 由 Proposed r3 变为 **Approved（依据本记录）**；实现可进入任务 #6。ADR 的三轮审查记录见 `docs/adr/ADR-001-review.md`。
2. **M1 完成门槛包含 M1-B**：M1 不再以“切换就绪（M1-A）”为终点，必须在**具名目标环境**完成停服、单写者交接、对账与回滚待命后，方可宣布 M1 完成。未取得具名目标与维护窗口前，M1 保持 M1-A 状态，不得宣布完成（也不得表述为已接管）。
3. **平台声明范围收窄为 Linux（Ubuntu LTS）**：darwin 仅开发/离线验证；Windows 保持“未验证”。M2 的 M2-PLATFORM 只对 Linux 主张支持，前提是 M2-PZ-LIVE 与启动向量对抗测试在 Linux 实机通过。
4. **Go 工具链**：允许经官方发行包安装到项目外目录；版本随后固定写入 `docs/migration/TOOLCHAIN.md`、`go.mod`、`ci`（三处一致），实现前必须完成（任务 #6）。

## 用户答复（第二轮，共 3 问）

| # | 问题 | 用户选择 | 影响 |
|---|---|---|---|
| 5 | M1-B 具名目标如何提供 | **暂无 Linux 主机，先停在 M1-A** | M1 本次终点回到 **M1-A（切换就绪）**；M1-B 仍属路线图完成门槛但**阻塞于目标主机**，M1 不得宣布“已完成迁移” |
| 6 | 迁移/回滚演练使用什么数据 | **允许生成代表性合成夹具** | 任务 #11/#13 可在本机生成含 `instance.json`、PZ INI（哨兵口令）、嵌套默认值、损坏与异常样本的合成 legacy 数据根，仅用于演练 |
| 7 | M2 实机验收环境 | **另给一台（补充说明）**（本次未附带具体信息） | #15（M2-PZ-LIVE / M2-PLATFORM）保持阻塞，等待主机与数据根信息 |

**综合结论**：本次可交付上限 = M1-A + M2 的隔离环境验收；实机平台声明（Linux）与 M1-B 接管均待用户提供主机后另行授权。

## 第三轮用户决定（2026-09-28，平台优先级变更）

| # | 用户决定 | 生效含义 |
|---|---|---|
| 8 | **Windows 是首先要支持的平台**（原先“仅 Linux 声明”作废） | ADR §1.6 平台表更新：Windows = 第一候选；Linux Ubuntu LTS = 第二候选；darwin 仍仅开发。支持声明仍需实机验收证据，不因优先级变化而提前声明 |
| 9 | 测试主机 Windows：`winssh.liubaitech.cn`（SSH，用户 admin）/ `wingame.liubaitech.cn`（RDP） | 作为 M2-PZ-LIVE / M2-PROCESS Windows / M1-A 隔离副本演练的目标；RDP 代理不可用，仅用 SSH |
| 10 | 安装位置：**G 盘新建文件夹** | 目标机安装根目录 `G:\gameserver-work`（Go 工具链 + 仓库副本 + 隔离数据根），详见 `docs/migration/WINDOWS-SETUP.md` |

**接入前置（当前阻塞）**：本机 `cloudflared access ssh --hostname winssh.liubaitech.cn` 返回 `websocket: bad handshake`，即 Cloudflare Access 客户端认证未完成（`~/.cloudflared` 不存在）。需用户在本机执行一次 `cloudflared access login winssh.liubaitech.cn` 完成浏览器登录；此后 `~/.cloudflared` 会缓存令牌，orchestrator 才能通过 ProxyCommand 建立 SSH。

**仍生效的旧决定**：M1-B（生产接管）未授权；任何真实副作用（SteamCMD 下载、PZ 安装/启动、端口监听、防火墙/权限变更）需 Target Manifest 完整 + 逐项授权；`data/` 与既有用户数据只读。

## 当前缺口（M1-B / M2 实机的前置输入，任务 #5 阻塞项）

只读核查结果：

- 本工作区 **没有任何部署配置**（无 Dockerfile/compose/ansible/systemd/部署脚本），无 CI 部署步骤。
- `data/servers/pz_01/` 只有空目录（`Zomboid/`、`server_files/`），**不存在 `instance.json`、PZ INI 或已安装服务端文件** → 本机没有可供迁移的真实遗留数据；迁移/回滚演练必须使用**代表性合成夹具**。
- SSH 管理器处于锁定状态，当前无法枚举是否存在可用的 Linux 目标主机。

因此 M1-B 与 M2-PZ-LIVE 需要用户提供：

| 需要的信息 | 用途 |
|---|---|
| 目标主机与访问方式（SSH 目标名或主机地址） | M1-B 切换与 M2 实机验收 |
| 数据根绝对路径（例如 `/srv/gameservers`） | 迁移提升的目标位置 |
| 实例 ID（例如 `pz_01`） | 所有权/锁/状态子树定位 |
| 维护窗口（日期/时段/时长） | M1-B 停服切换 |
| 该主机是否同时承担 M2 实机 PZ 验收 | 复用或另给环境 |

在取得上述信息前：任务 #6–#13（工具链、Go 实现、验收文档、M1-A 验证）**不受阻塞**；#14（M2 离线可完成部分）不受阻塞；#15（M2 实机）与 M1-B **保持阻塞**。

任务 #5 的状态：**部分完成**（ADR 与平台/工具链决策已取得并生效；M1-B 目标与 M2 实机环境信息仍未提供）。

## 边界声明

- 本记录只完成决策与缺口登记，**未执行**任何接管、部署、下载或数据变更动作。
- 用户既有脏改动（`docs/SYSTEM_DESIGN_AND_ARCHITECTURE.md`、`.maestroignore`、`.workflow/`、`Frameworks`）未被触碰。
