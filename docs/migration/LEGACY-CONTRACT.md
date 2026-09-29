# 旧版（Python/FastAPI）行为契约与数据边界

> 已批准 Plan（handoff key `86c3e976a061fc28b352f996708826a95c395a6600595d2b4cf9f13f1b8fc8f0`）阶段 1 产出。
> 用途：Go 兼容实现与 M2 验收矩阵（M2-API / M2-AUTH / M2-WS / M2-UI / M2-CONFIG / M2-SECRET）的逐项对齐依据。
> 基线提交：`ebacf29084e63afb505044ae69957c88a951e772`（与 CI run 36383916726 一致，见 `BASELINE.md`）。
> **本文件只写字段名与脱敏示例，不含任何真实口令。**

## 1. 会话与认证

| 项 | 基线行为 | 位置 |
|---|---|---|
| Cookie 名 | `gameserver_session` | `api/routes.py:22` |
| 令牌格式 | `"{unix_ts}.{hmac_sha256_hex(ts, secret)}"` | `api/routes.py:53-54,152-153` |
| 签名密钥 | 每次 `create_app` 生成 32 字节随机密钥（进程内存，重启即失效） | `api/routes.py:132` |
| TTL | `SESSION_TTL_SECONDS = 12*60*60`（12h）；未来时间戳视为无效 | `api/routes.py:23,57-66` |
| Cookie 属性 | `httponly=True`、`same_site="strict"`、`path="/"`、`max_age=43200`、`secure=(GAMESERVER_COOKIE_SECURE=="1")` | `api/routes.py:156-164` |
| 口令来源 | `admin_password` 参数优先，否则 `GAMESERVER_ADMIN_PASSWORD` | `api/routes.py:131` |
| 未配置口令 | 登录与所有受保护操作返回 **503**「尚未配置 GAMESERVER_ADMIN_PASSWORD」 | `api/routes.py:148-149,175-176` |
| 口令比较 | `hmac.compare_digest`，失败 **401**「管理员密码错误」 | `api/routes.py:150-151` |
| `/api/auth/status` | `{"authenticated": bool(configured_password and valid_session)}`（未配置口令时恒为 false，不报错） | `api/routes.py:142-144` |
| `/api/auth/logout` | `{"authenticated": false}` + 删除 Cookie；**服务端不吊销令牌** | `api/routes.py:167-172` |
| HTTP Origin 校验 | 仅当请求带 `Origin` 时比较 `netloc` 与 `Host`，不符 **403**；**缺失 Origin 视为通过** | `api/routes.py:177-182` |
| WS Origin 校验 | 同上，缺失 Origin 视为通过；失败以 code **1008** 关闭 | `api/routes.py:347-351` |
| WS 会话校验 | accept 之前校验；无口令或会话无效 → code **1008** | `api/routes.py:344-345` |

## 2. HTTP 路由契约

请求体模型继承 `StrictModel`（`ConfigDict(extra="forbid")`，`api/routes.py:27-28`），未知字段 → **422**。

| # | 方法/路径 | 位置 | 认证 | 成功响应 | 错误语义 |
|---|---|---|---|---|---|
| 1 | GET `/api/auth/status` | `:142` | 否 | 200 `{authenticated}` | — |
| 2 | POST `/api/auth/login` | `:146` | 否（body `{password}`） | 200 `{authenticated:true}` + Set-Cookie | 503 未配置口令；401 口令错误；422 缺字段/多余字段 |
| 3 | POST `/api/auth/logout` | `:167` | 否 | 200 `{authenticated:false}` + 清 Cookie | — |
| 4 | GET `/api/status` | `:185` | 是 | 200 `get_summary()` + `variables`(脱敏) + `install_task` | 401/403/503 |
| 5 | GET `/api/templates` | `:193` | 是 | 200 模板摘要数组（id/name/category/icon/author/version/description/supported_os/app_id） | 401/403/503 |
| 6 | POST `/api/server/install` | `:198` | 是 | **202** `{message,status:"INSTALLING"}` | 409 已有安装任务；409 服务器运行中；401/403/503 |
| 7 | GET `/api/server/install` | `:229` | 是 | 200 `install_state` = `{status,progress,message,error}`（status ∈ IDLE/INSTALLING/COMPLETED/FAILED） | 401/403/503 |
| 8 | POST `/api/server/start` | `:234` | 是 | 200 `{message,running:true}`；已运行时 200「已在运行中」 | 409 安装中；400 未安装；500 启动失败 |
| 9 | POST `/api/server/stop` | `:247` | 是 | 200 `{message,success:true}` | 500 停止失败 |
| 10 | POST `/api/server/restart` | `:255` | 是 | 200 `{message,success:true}` | 500 停止失败（取消重启）；400 未安装；500 启动失败 |
| 11 | POST `/api/server/kill` | `:266` | 是 | 200 `{message,success:bool}`（始终 200） | — |
| 12 | POST `/api/server/command` | `:272` | 是（body `{command}` 1..4096） | 200 `{success:bool}` | 400 未运行 |
| 13 | GET `/api/server/logs?limit=` | `:280` | 是 | 200 `{logs:[...]}`，limit 夹在 1..1000，默认 150 | 401/403/503 |
| 14 | GET `/api/server/config` | `:285` | 是 | 200 `{variables,ports,fields,template:{id,name}}`；`fields` 仅 `user_editable`，密码字段 `value:null` 且移除 `default` | 401/403/503 |
| 15 | POST `/api/server/config` | `:305` | 是（body `{variables?,ports?}`） | 200 `{message,state}`（state.variables 脱敏） | 422 校验失败（见 §3） |
| 16 | POST `/api/server/mods` | `:318` | 是（`{workshop_id,mod_name?}`） | 200 `{message,mods}`；仅登记不下载 | 422 workshop_id 非数字 |
| 17 | DELETE `/api/server/mods/{workshop_id}` | `:326` | 是 | 200 `{message,mods}` | — |
| 18 | POST `/api/server/renew` | `:332` | 是（`{months}` 1..24） | 200 `{message,billing}`；`expire_days_left += months*30`，status=ACTIVE，并广播日志行 | 422 范围外 |
| 19 | GET `/` | `:389` | 否 | 200 `static/index.html`；文件缺失时 200 JSON `{"message":"Gameserver Backend Running"}` | — |
| 20 | `/static/*` | `:387` | 否 | `StaticFiles` 直出 | — |

### 2.1 配置校验规则（`_validate_config`，`:78-112`）

- 仅允许模板中声明且 `user_editable: true` 的键（否则 422「不允许修改配置项」）。
- `string`：必须字符串；`SERVER_NAME` 需匹配 `^[A-Za-z0-9_-]{1,64}$`；长度 ≤256。
- `password`：字符串 ≤256；`required` 且为空 → 422。
- `number`：必须是 `int` 且非 `bool`；须落在 `validation.min/max` 内。
- `boolean`：必须是 `bool`。
- `ports`：键必须在模板 `ports` 中声明；值 1..65535。

### 2.2 脱敏规则（`_redacted_variables`，`:69-76`）

| 键 | 输出 |
|---|---|
| `SERVER_PASSWORD` | 恒为 `""` |
| `ADMIN_PASSWORD` | 真值 → `None`；假值 → `""` |
| 其他 | 原值 |

模板与变量定义见 `templates/project_zomboid.yaml`（变量：SERVER_NAME/SERVER_PASSWORD/ADMIN_PASSWORD/MAX_PLAYERS/PVP_ENABLED/PUBLIC_SERVER/OPEN_REGISTRATION/PAUSE_EMPTY；端口：SERVER_PORT 16261、DIRECT_PORT 16262；`app_id=380870`；`supported_os=[windows,linux,darwin]`）。

### 2.3 错误信封与 UI 依赖字符串（**强制契约**）

错误响应体统一为 `{"detail": <string>}`（FastAPI `HTTPException` 默认格式）。UI 直接消费该字段（`static/index.html` 的 `api()`：`throw new Error(data.detail || \`请求失败 (${response.status})\`)`），因此 **Go 实现必须复现 `detail` 字段与以下字符串**，否则 M2-UI 的未登录/错误路径会失效。

| 触发条件 | 状态码 | 字符串 | 位置 |
|---|---|---|---|
| 未配置控制面口令 | 503 | `尚未配置 GAMESERVER_ADMIN_PASSWORD` | `api/routes.py:149,176` |
| 登录口令错误 | 401 | `管理员密码错误` | `:151` |
| 跨站 Origin | 403 | `跨站请求已拒绝` | `:181` |
| 会话无效/缺失 | 401 | `请先登录` | `:183` |
| 已有安装任务 | 409 | `已有安装任务在运行中` | `:203` |
| 服务器运行中安装 | 409 | `服务器运行中，请先关机后再更新` | `:205` |
| 安装中启动 | 409 | `正在安装中，请稍候启动` | `:240` |
| 未安装即启动 | 400 | `游戏服务端未安装，请先安装服务端` | `:242` |
| 启动失败 | 500 | `服务器启动失败，请检查控制台输出` | `:244` |
| 停止失败 | 500 | `服务器停止失败` | `:252` |
| 重启时停止失败 | 500 | `停止服务器失败，已取消重启` | `:259` |
| 重启时未安装 | 400 | `游戏服务端未安装` | `:261` |
| 重启启动失败 | 500 | `服务器重启失败，请检查控制台输出` | `:263` |
| 未运行时发指令 | 400 | `服务器未运行，无法发送控制台指令` | `:276` |
| 配置/端口校验失败 | 422 | 允许具体文案演进，但必须是字符串且以 `detail` 返回 | `:85-112` |

**UI 关键分支**：`static/index.html` 在 `loadAll()` 中以 `error.message === '请先登录'` 判定会话过期并回到登录页——该字符串属于兼容契约，不得改写。

**附加响应字段规则**：允许新增字段（例如后续的 `ownership`、`ready`），但不得重命名或移除上表字段、不得改变其类型；新增字段必须在 `docs/adr/ADR-001-go-backend.md` 与 M2 计划中登记。

### 2.4 UI 请求形态约束（Go 必须容忍）

| 约束 | 证据 | 要求 |
|---|---|---|
| 所有请求（含 GET）携带 `Content-Type: application/json` | `static/index.html` `api()` 统一设置 headers | Go 不得因 GET 带该头而报错 |
| `action()` 对 install/start/stop/restart 发送 body `{}` | `action(path,label)` 使用 `body:'{}'` | 这些无请求模型的路由必须接受空 JSON 对象，不得返回 422 |
| 浏览器同源 POST 会带 `Origin`；同源 GET 通常不带 | 浏览器行为 + `api()` 同源 fetch | 与 §2.3 / D1 的 Origin 策略一致（有副作用请求缺 Origin 拒绝；只读可选） |
| `/api/status` 每 2.5s 轮询一次 | `setInterval(...,2500)` | 响应必须保持轻量且字段不变 |
| `install_task.progress` 必须为数字 | `task.progress.toFixed(1)` | 不得返回字符串或省略该字段 |
| 配置字段形状 | `renderField()` 读取 `label/key/type/value` 与 `validation.min/max` | 字段名与类型不得变更 |

## 3. WebSocket 契约 `/ws/console`

| 阶段 | 行为 | 位置 |
|---|---|---|
| 鉴权 | accept 之前：无口令 / 会话无效 / Origin 不符 → close **1008**，不接收任何消息 | `:344-351` |
| 初始回放 | accept 后立即发送最近 **100** 行：`{"type":"log","data":"<line>"}` | `:353-355` |
| 追加日志 | 订阅 supervisor listener（`add_listener`）→ 每条 `{"type":"log","data":...}` | `:356-368` |
| 客户端消息 | 文本 JSON；`{"type":"input","data":...}` → `send_input`；`{"type":"ping"}` → `{"type":"pong"}`；其他类型忽略 | `:369-381` |
| 非 JSON 文本 | 视为输入原文（`{"type":"input","data":<原文>}` 等价） | `:372-375` |
| **未运行实例** | 仍调用 `send_input`，内部返回 False，**WS 层不回错误、不关闭** | `:376-377, core/process_supervisor.py:134-138` |
| 断开清理 | 取消 sender task、`remove_listener` | `:382-384` |
| 服务端→客户端其他帧 | 无（无状态推送、无安装进度帧） | — |

## 4. 静态 UI 契约（`static/index.html`，46 行）

实际调用面（Go 实现不得破坏）：

| 调用 | 位置 | 说明 |
|---|---|---|
| `GET /api/auth/status` → 决定登录页/仪表盘 | `static/index.html:28` | 失败即显示登录页 |
| `POST /api/auth/login` `{password}` | `:29` | 成功后清空输入框并进仪表盘 |
| `POST /api/auth/logout` | `:30` | 先 `socket.close()` 再登出 |
| `GET /api/status` | `:32` | 渲染 status/is_installed/platform/ports/pid/cpu/memory/uptime/install_task |
| `GET /api/server/config` | `:34` | `fields` 动态渲染；密码输入留空表示不改；端口输入框读 `ports.SERVER_PORT/DIRECT_PORT` |
| `POST /api/server/config` `{variables}` / `{ports}` | `:40-41` | 仅提交用户填写的键；密码仅在非空时提交 |
| `GET /api/server/logs?limit=200` | `:35` | 文本拼接 |
| `POST /api/server/install`、`start`、`stop`、`restart` | `:39` | `action()` 统一错误提示 |
| `POST /api/server/command` `{command}` | `:42` | 错误写入 `#install-info` |
| `WS /ws/console` | `:37` | 断线后每 2.5s 自动重连；仅处理 `type==="log"` |

**UI 未调用**（仍属 API 契约，但无 UI 依赖）：`GET /api/templates`、`GET /api/server/install`、`POST /api/server/kill`、`POST/DELETE /api/server/mods`、`POST /api/server/renew`。

**Origin 兼容性事实：**UI 的同源 `fetch` 对非 GET 方法会携带 `Origin`，因此计划中「有副作用请求缺失 Origin 即拒绝」不会破坏既有 UI 流程；同源 GET 通常不带 `Origin`，故只读接口需允许缺失 Origin（跨源仍拒绝）。

### 4.1 r3 新增 UI 调用面（2026-09-29）

| 调用 | 用途 | 备注 |
|---|---|---|
| `GET /api/templates` | 选择游戏与**服务端版本**（`versions[]`：label/branch/build_id/default/evidence_ref） | legacy UI 未调用；纯追加 |
| `POST /api/server/install`（`{"version":"<branch>"}`） | 指定分支下载/校验；`{}` 与空体保持兼容 | 未知分支 422「所选服务端版本不在模板清单中」，不启动安装 |
| `GET /api/server/config` → `options[]` / `groups[]` | 目录驱动的全量配置项视图（416 项），值以文件回读为准 | `fields[]` 与既有 8 项投影不变 |
| `POST /api/server/config`（`{"options":{…}}`） | 选项写路径：ro/hidden → 409、未知/类型/范围/枚举 → 422、secret 留空=保持 | 与 `{"variables":…}` 互不覆盖（所有权矩阵） |

UI 调用面不再以固定计数核对；由 `TestStaticUIContract` 动态断言 endpoint 集合 ⊆ 路由集合。

## 5. 持久化与数据/副作用边界

| 边界 | 路径/位置 | 语义 | 位置 |
|---|---|---|---|
| 实例状态 | `data/servers/pz_01/instance.json` | 整份 JSON 直接 `json.dump`（**非原子**）；读取时对 dict 做**一层嵌套合并**，顶层键直接覆盖 | `core/instance_manager.py:69-90` |
| 状态默认值 | 代码内默认：variables/ports/quota_gb/mods/billing | 新实例 `ADMIN_PASSWORD=secrets.token_urlsafe(24)`；`SERVER_NAME=servertest`；端口 16261/16262 | `core/instance_manager.py:41-67` |
| 状态读取保护 | SERVER_NAME 不匹配 `^[A-Za-z0-9_-]{1,64}$` 时回落 `servertest` | `core/instance_manager.py:79-81` |
| PZ 主配置 | `data/servers/pz_01/Zomboid/Server/<SERVER_NAME>.ini` | 按行改写：更新受管键、**保留注释与未知键**、缺失键追加；直接 `writelines`（**非原子**） | `core/zomboid_config.py:26-31,50-90` |
| INI 默认值 | `DEFAULT_SETTINGS`（含 RCONPort/RCONPassword 等未暴露键） | 文件不存在时返回默认值副本 | `core/zomboid_config.py:6-24,34-36` |
| 游戏服务端文件 | `data/servers/pz_01/server_files/` | SteamCMD `+force_install_dir` 目标；install 后对 unix 平台 `chmod 0755` `*.sh` 与 `ProjectZomboid64` | `core/instance_manager.py:30-31,158-164` |
| SteamCMD | `data/steamcmd/` | 首次自动下载/解压（windows zip / darwin tar.gz / linux tar.gz）；可执行 `steamcmd.exe`/`steamcmd.sh` | `core/steamcmd_service.py:32-83` |
| 磁盘用量 | 递归统计 `instance_dir` 字节数 | 观测值，非配额 | `core/instance_manager.py:92-101` |
| 日志 | **仅内存**（deque，默认 1000 行），`data/logs/` 目录被创建但未用于持久化 | 重启即丢失 | `core/process_supervisor.py:16-17`；`config.py:27-28` |
| 模板 | `templates/*.yaml`（`TemplateManager.load_all`，`get_template` 未命中时重载） | 元数据 + 环境/端口/变量/绑定/生命周期 | `core/template_manager.py:14-29` |

### 5.1 r3/r4 增量：受管键所有权与 SandboxVars（非 legacy 行为）

| 项 | legacy | Go 版（r3/r4） | 依据 |
|---|---|---|---|
| 可写集合 | 固定 10 个受管键 | 由经校验的选项目录（`writable=rw`）决定；`ro/hidden` 拒绝并给精确文案 | ADR §5.4 D9 |
| 写入者 | 变量页与配置页可写同一批键 | **一物理键一写入者**：变量页自有 10 键 + mods 两键归 mods API；`ApplyGameConfig` 只写自有键；跨路径写入 409 | ADR §5.4 D9、实施 #25 |
| SandboxVars | 不受管、不备份 | 受管文件：规范化写入、注释与未知键保留、`.bak1..3` 轮转、逐字节读回、失败关闭；纳入备份范围 | §8.1、ADR §1.4/§4.6 |
| 秘密 | `Password`、`RCONPassword` 明文可读 | 值不回显（读回 `null`）、留空=保持当前值、字典内标 `secret` | ADR §1.7、M2-SECRET |

## 6. 秘密边界

| 秘密 | 存放 | 暴露面 |
|---|---|---|
| 控制面口令 `GAMESERVER_ADMIN_PASSWORD` | 进程环境变量 | 仅用于登录比较；不出现在响应/日志 |
| 游戏管理员口令 `ADMIN_PASSWORD` | `instance.json`（明文）+ 启动 argv `-adminpassword=<value>` | 响应中被脱敏；**argv 对同机同权限进程可见**；命令回显已做 `[REDACTED]` 处理（`core/process_supervisor.py:55-64`） |
| 入服口令 `SERVER_PASSWORD` | `instance.json`（明文）+ PZ INI `Password=` | 响应中恒为空串 |
| 会话密钥 | 仅进程内存 | 重启失效 |
| SteamCMD | 使用 `anonymous` 登录，无存储凭据 | — |

## 7. 进程内（非持久）状态

| 状态 | 位置 | 影响 |
|---|---|---|
| `session_secret` | `api/routes.py:132` | 重启后旧 Cookie 全部失效 |
| `install_state` / `install_task` | `api/routes.py:133-134` | 重启后安装状态丢失为 IDLE；不持久 |
| `SteamCMDService.is_busy/current_progress/current_status_text` | `core/steamcmd_service.py:22-24` | 单进程内互斥；**无跨进程互斥** |
| `ProcessSupervisor.process/pid/logs/listeners/status/started_at/exit_code` | `core/process_supervisor.py:16-25` | 重启后无法得知既有游戏进程归属 |
| `TemplateManager.templates` | `core/template_manager.py:11` | 可重载 |

## 8. 显式安全差异（Go 版必须变更，且不得称为「完全兼容」）

| # | 现状 | 目标 | 理由 |
|---|---|---|---|
| D1 | 缺失 `Origin` 时放行（HTTP 与 WS 均是） | 有副作用请求缺失 Origin → 拒绝；只读 GET 允许缺失但拒绝跨源；WS 必须匹配 Origin | CSRF/跨站 WebSocket 劫持；与 UI 同源 fetch 行为兼容（见 §4） |
| D2 | 登出仅删浏览器 Cookie，服务端令牌仍有效 | 登出后旧令牌请求被拒（服务端吊销或短时缓存）；重启策略按 ADR（建议启动即轮换密钥） | 令牌重放 |
| D3 | WS 在实例未运行时仍接受 `input`（静默失败） | 未运行时明确拒绝，不静默吞掉 | 状态与错误语义一致性 |
| D4 | `install_state` 无取消/超时语义；子进程回收无证据 | 提供 deadline/受控 shutdown 取消与子进程 reap；不新增公开 cancel 路由（除非另行范围审查） | M2-INSTALL 验收要求 |
| D5 | `instance.json` 与 INI 直写（无原子性/失败返回） | 临时文件 + fsync + rename，失败必须返回失败或 `recovery required`，不得假成功 | M2-CONFIG 损坏/写入失败验收 |
| D6 | 无跨进程所有权 | OS 级独占锁 + 所有权记录 + 启动对账 + 失败关闭 | M2-SINGLEWRITER / M2-RESTART |
| D7 | 三平台均经脚本/解释器中转启动（`.bat` / `/bin/bash start-server.sh`）且秘密经 argv | 按 ADR 逐 OS 声明启动向量；脚本中转必须做特殊字符/引号/参数注入对抗测试；**管理员密码不得经解释器重解析** | 启动向量注入（M2-PROCESS） |


## 8.1 Go 新增受管文件（非 legacy 行为）

`<instance_root>/Zomboid/Server/<name>_SandboxVars.lua`：legacy Python 实现**从不读写**该文件（`ZomboidConfigHandler` 只处理 `Server/<name>.ini`）。Go 版本按 `catalogs/<template>.options.yaml`（由 `tools/pzoptions` 从厂商产物生成）托管其顶层与嵌套赋值：规范化写入、保留注释与未受管键、`.bak1..3` 轮转、读回校验、非法值失败关闭。该差异属**新增能力**，不改变既有 INI/状态契约。

## 9. Go 兼容清单（逐项勾选）

- [ ] 20 个路由/挂载点全部存在且方法、状态码、响应字段一致（差异仅限 §8）。
- [ ] `StrictModel` 等价：未知字段 422；`command` 1..4096；`months` 1..24；`workshop_id` 数字校验。
- [ ] 配置校验 5 类规则与端口范围一致；`fields` 仅 `user_editable`；密码字段不返回 default。
- [ ] 脱敏规则与 §2.2 完全一致。
- [ ] WS：1008 关闭、100 行回放、log/input/ping/pong、非 JSON 文本按输入处理、断开清理。
- [ ] 静态资源：`/`、`/static/*` 可服务；UI 调用面行为不变（**r3 起改为动态断言**：解析 `static/index.html` 的端点点集 ⊆ 路由集，不再硬编码「10 处调用点 / 13 端点」，见 §4.1 与 ADR §1.3）。
- [ ] INI：保留未知键与注释、只更新受管键、空 Mod 列表写空串（r3 起「受管键」= 目录授权集合且按所有权切分，见 §5.1）。
- [ ] 错误信封 `{"detail": <string>}` 与 §2.3 全部字符串逐字一致（含 UI 分支依赖的 `请先登录`）。
- [ ] 容忍 GET 带 `Content-Type: application/json`、无模型 POST 接受 `{}`（§2.4）。
- [ ] 模板 `supported_os` 等元数据按原样返回（兼容数据），但**不作为平台支持声明**。
- [ ] 日志：内存环形缓冲（容量与重启即失语义按 ADR 记录）。
