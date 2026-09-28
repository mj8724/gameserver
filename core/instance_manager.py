import asyncio
import json
import os
import re
import secrets
import shutil
from pathlib import Path
from typing import Dict, Any, Optional, Callable

from config import SERVERS_DIR, PLATFORM
from core.template_manager import TemplateManager
from core.steamcmd_service import SteamCMDService
from core.process_supervisor import ProcessSupervisor
from core.zomboid_config import ZomboidConfigHandler

class InstanceManager:
    def __init__(
        self,
        instance_id: str = "pz_01",
        template_manager: Optional[TemplateManager] = None,
        steamcmd_service: Optional[SteamCMDService] = None,
        supervisor: Optional[ProcessSupervisor] = None,
    ):
        self.instance_id = instance_id
        self.template_manager = template_manager or TemplateManager()
        self.steamcmd_service = steamcmd_service or SteamCMDService()
        self.supervisor = supervisor or ProcessSupervisor()

        self.instance_dir = SERVERS_DIR / self.instance_id
        self.install_dir = self.instance_dir / "server_files"
        self.cache_dir = self.instance_dir / "Zomboid"
        self.state_file = self.instance_dir / "instance.json"

        self.instance_dir.mkdir(parents=True, exist_ok=True)
        self.install_dir.mkdir(parents=True, exist_ok=True)
        self.cache_dir.mkdir(parents=True, exist_ok=True)

        self.state: Dict[str, Any] = {
            "instance_id": self.instance_id,
            "name": "Project Zomboid Dedicated Server",
            "template_id": "project_zomboid",
            "variables": {
                "SERVER_NAME": "servertest",
                "SERVER_PASSWORD": "",
                "ADMIN_PASSWORD": secrets.token_urlsafe(24),
                "MAX_PLAYERS": 16,
                "PVP_ENABLED": True,
                "PUBLIC_SERVER": True,
                "OPEN_REGISTRATION": True,
                "PAUSE_EMPTY": True,
            },
            "ports": {
                "SERVER_PORT": 16261,
                "DIRECT_PORT": 16262,
            },
            "quota_gb": 30.0,
            "mods": {
                "workshop_ids": [],
                "mod_names": [],
            },
            "billing": {
                "status": "ACTIVE",
                "package_name": "帕鲁/僵尸生存 4核16G 畅玩版",
                "expire_days_left": 28,
            }
        }
        self.load_state()

    def load_state(self):
        if self.state_file.exists():
            try:
                with open(self.state_file, "r", encoding="utf-8") as f:
                    saved = json.load(f)
                    for key, value in saved.items():
                        if isinstance(value, dict) and isinstance(self.state.get(key), dict):
                            self.state[key].update(value)
                        else:
                            self.state[key] = value
                    server_name = self.state.get("variables", {}).get("SERVER_NAME", "servertest")
                    if not isinstance(server_name, str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,64}", server_name):
                        self.state["variables"]["SERVER_NAME"] = "servertest"
            except Exception as e:
                print(f"[InstanceManager] 加载状态失败: {e}")

    def save_state(self):
        try:
            with open(self.state_file, "w", encoding="utf-8") as f:
                json.dump(self.state, f, indent=2, ensure_ascii=False)
        except Exception as e:
            print(f"[InstanceManager] 保存状态失败: {e}")

    def get_disk_usage_mb(self) -> float:
        total_bytes = 0
        if self.instance_dir.exists():
            for root, _, files in os.walk(self.instance_dir):
                for f in files:
                    try:
                        total_bytes += os.path.getsize(os.path.join(root, f))
                    except OSError:
                        pass
        return round(total_bytes / (1024 * 1024), 1)

    def is_installed(self) -> bool:
        # Check for core server executables in install_dir
        if PLATFORM == "windows":
            return (self.install_dir / "StartServer64.bat").exists() or (self.install_dir / "ProjectZomboid64.exe").exists()
        elif PLATFORM == "darwin":
            return (self.install_dir / "ProjectZomboid64").exists() or (self.install_dir / "start-server.sh").exists()
        else:
            return (self.install_dir / "start-server.sh").exists() or (self.install_dir / "ProjectZomboid64").exists()

    def sync_config(self):
        server_name = self.state["variables"].get("SERVER_NAME", "servertest")
        ini_path = ZomboidConfigHandler.get_ini_path(self.cache_dir, server_name)

        updates = {
            "Public": self.state["variables"].get("PUBLIC_SERVER", True),
            "Password": self.state["variables"].get("SERVER_PASSWORD", ""),
            "MaxPlayers": self.state["variables"].get("MAX_PLAYERS", 16),
            "PVP": self.state["variables"].get("PVP_ENABLED", True),
            "Open": self.state["variables"].get("OPEN_REGISTRATION", True),
            "PauseEmpty": self.state["variables"].get("PAUSE_EMPTY", True),
            "DefaultPort": self.state["ports"].get("SERVER_PORT", 16261),
            "UDPPort": self.state["ports"].get("DIRECT_PORT", 16262),
        }

        updates["WorkshopItems"] = ";".join(self.state["mods"]["workshop_ids"])
        updates["Mods"] = ";".join(self.state["mods"]["mod_names"])

        ZomboidConfigHandler.write_config(ini_path, updates)
        self.supervisor.broadcast_line(f"[Instance] 已将配置同步写入: {ini_path.name}")

    async def install(
        self,
        validate: bool = True,
        on_log: Optional[Callable[[str], None]] = None,
        on_progress: Optional[Callable[[float, str], None]] = None
    ) -> bool:
        template = self.template_manager.get_template(self.state["template_id"])
        app_id = "380870"
        if template and "steam" in template:
            app_id = str(template["steam"].get("app_id", "380870"))

        def logger(msg: str):
            self.supervisor.broadcast_line(msg)
            if on_log:
                on_log(msg)

        success = await self.steamcmd_service.install_or_update(
            app_id=app_id,
            install_dir=self.install_dir,
            validate=validate,
            on_log=logger,
            on_progress=on_progress,
        )

        # Grant executable permissions on unix-like OS
        if success and PLATFORM != "windows":
            for sh_file in self.install_dir.glob("*.sh"):
                sh_file.chmod(0o755)
            pz_bin = self.install_dir / "ProjectZomboid64"
            if pz_bin.exists():
                pz_bin.chmod(0o755)

        return success

    async def start(self) -> bool:
        if self.supervisor.is_running():
            self.supervisor.broadcast_line("[Instance] 服务器已经在运行中！")
            return False

        if not self.is_installed():
            self.supervisor.broadcast_line("[Instance] 错误: 游戏服务端尚未下载或安装，请先点击'安装/更新服务端'！")
            return False

        # First, sync configuration file
        self.sync_config()

        server_name = self.state["variables"].get("SERVER_NAME", "servertest")
        admin_pass = self.state["variables"].get("ADMIN_PASSWORD", "")
        if not admin_pass:
            self.supervisor.broadcast_line("[Instance] 错误: 请先设置游戏管理员密码。")
            return False

        # Build execution command based on platform
        cache_dir_arg = f"-cachedir={self.cache_dir.resolve()}"
        name_arg = f"-servername={server_name}"
        admin_arg = f"-adminpassword={admin_pass}"

        cmd = []
        if PLATFORM == "windows":
            bat_path = self.install_dir / "StartServer64.bat"
            if bat_path.exists():
                cmd = ["cmd.exe", "/c", str(bat_path.resolve()), cache_dir_arg, name_arg, admin_arg]
            else:
                exe_path = self.install_dir / "ProjectZomboid64.exe"
                cmd = [str(exe_path.name), cache_dir_arg, name_arg, admin_arg]
        elif PLATFORM == "darwin":
            # On macOS, check for start-server.sh or ProjectZomboid64
            sh_path = self.install_dir / "start-server.sh"
            if sh_path.exists():
                cmd = ["/bin/bash", str(sh_path.name), cache_dir_arg, name_arg, admin_arg]
            else:
                bin_path = self.install_dir / "ProjectZomboid64"
                cmd = [str(bin_path.name), cache_dir_arg, name_arg, admin_arg]
        else: # linux
            sh_path = self.install_dir / "start-server.sh"
            cmd = ["/bin/bash", str(sh_path.name), cache_dir_arg, name_arg, admin_arg]

        # Launch process via supervisor
        env = {
            "LD_LIBRARY_PATH": f"{self.install_dir}/linux64:{self.install_dir}:{os.environ.get('LD_LIBRARY_PATH', '')}"
        }
        return await self.supervisor.start(cmd=cmd, cwd=self.install_dir, env=env)

    async def stop(self) -> bool:
        template = self.template_manager.get_template(self.state["template_id"]) or {}
        lifecycle = template.get("lifecycle", {})
        stop_cmd = lifecycle.get("stop_command", "quit")
        timeout = lifecycle.get("stop_timeout_seconds", 30)

        return await self.supervisor.stop(stop_command=stop_cmd, timeout=timeout)

    def kill(self) -> bool:
        return self.supervisor.kill()

    def update_variables(self, vars_update: Dict[str, Any]):
        self.state["variables"].update(vars_update)
        self.save_state()
        self.sync_config()

    def update_ports(self, ports_update: Dict[str, int]):
        self.state["ports"].update(ports_update)
        self.save_state()
        self.sync_config()

    def add_workshop_mod(self, workshop_id: str, mod_name: Optional[str] = None):
        wid = workshop_id.strip()
        if wid and wid not in self.state["mods"]["workshop_ids"]:
            self.state["mods"]["workshop_ids"].append(wid)
        if mod_name:
            mname = mod_name.strip()
            if mname and mname not in self.state["mods"]["mod_names"]:
                self.state["mods"]["mod_names"].append(mname)
        self.save_state()
        self.sync_config()

    def remove_workshop_mod(self, workshop_id: str):
        if workshop_id in self.state["mods"]["workshop_ids"]:
            idx = self.state["mods"]["workshop_ids"].index(workshop_id)
            self.state["mods"]["workshop_ids"].remove(workshop_id)
            if idx < len(self.state["mods"]["mod_names"]):
                self.state["mods"]["mod_names"].pop(idx)
        self.save_state()
        self.sync_config()

    def get_summary(self) -> Dict[str, Any]:
        metrics = self.supervisor.get_metrics()
        used_mb = self.get_disk_usage_mb()
        quota_gb = self.state.get("quota_gb", 30.0)
        disk_ratio = round((used_mb / (quota_gb * 1024)) * 100, 1) if quota_gb > 0 else 0.0

        status = metrics["status"]
        if self.steamcmd_service.is_busy:
            status = "INSTALLING"

        return {
            "instance_id": self.instance_id,
            "name": self.state["name"],
            "template_id": self.state["template_id"],
            "is_installed": self.is_installed(),
            "status": status,
            "running": metrics["running"],
            "pid": metrics.get("pid"),
            "cpu_percent": metrics["cpu_percent"],
            "memory_mb": metrics["memory_mb"],
            "uptime_seconds": metrics["uptime_seconds"],
            "disk": {
                "used_mb": used_mb,
                "quota_gb": quota_gb,
                "usage_percent": disk_ratio,
            },
            "ports": self.state["ports"],
            "variables": self.state["variables"],
            "mods": self.state["mods"],
            "billing": self.state["billing"],
            "steamcmd": {
                "busy": self.steamcmd_service.is_busy,
                "progress": self.steamcmd_service.current_progress,
                "status_text": self.steamcmd_service.current_status_text,
            },
            "platform": PLATFORM,
        }
