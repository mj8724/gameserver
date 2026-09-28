# GameServer 控制台（Project Zomboid 原型）

这是一个 Python/FastAPI 的单机 Project Zomboid Dedicated Server 管理原型，可通过浏览器查看状态、管理安装任务、编辑基础配置、启停服务并查看控制台日志。当前只面向单机单实例，不包含用户体系、真实 Workshop 下载、支付、多节点或系统级资源配额。

## 环境要求

- Python 3.11 或更高版本
- 网络连接（首次安装 SteamCMD/游戏服务端时需要）
- Project Zomboid 专用服务端在目标操作系统上的兼容性仍需实机验证；Windows 行为需在 Windows 环境完成联调后确认

## 安装与启动

```bash
python -m venv .venv
source .venv/bin/activate  # Windows PowerShell: .venv\Scripts\Activate.ps1
python -m pip install -r requirements.txt
```

设置唯一管理员口令后启动。必须使用你自己的强口令，不要复用游戏管理员密码：

```bash
export GAMESERVER_ADMIN_PASSWORD='替换成强口令'
python main.py --browser
```

Windows PowerShell：

```powershell
$env:GAMESERVER_ADMIN_PASSWORD = '替换成强口令'
python main.py --browser
```

默认只监听 `127.0.0.1:8769`。可用 `GAMESERVER_HOST`、`GAMESERVER_PORT` 或 `--host`、`--port` 覆盖；如监听非本机地址，请确保通过 HTTPS 反向代理或可信私有网络访问。设置 `GAMESERVER_COOKIE_SECURE=1` 可要求会话 Cookie 仅经 HTTPS 发送。

## 数据与测试

服务端文件及实例状态写入项目下的 `data/`；模板位于 `templates/`。安装测试依赖并运行离线测试：

```bash
python -m pip install -r requirements-dev.txt
python -m pytest -q
```

测试不会下载 SteamCMD 或启动真实游戏进程。真实安装、Windows 批处理启动、服务端就绪状态及端口可达性仍需要 Windows 实机验收。

## 当前边界

- 控制面登录口令通过 `GAMESERVER_ADMIN_PASSWORD` 配置；游戏管理员密码需在页面配置中主动设置，新实例不会使用仓库内的固定默认值。
- PZ Workshop Mod 接口只登记 Workshop ID/Mod 名称并同步配置，不会下载 Mod。
- 续费功能仅为本地状态模拟；磁盘/CPU/内存展示不构成资源硬隔离。
- 当前仅实现一个 `pz_01` 实例，不支持多用户、多实例、SQLite、远程节点或支付。
