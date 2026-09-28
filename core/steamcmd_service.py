import asyncio
import os
import re
import shutil
import sys
import tarfile
import urllib.request
import zipfile
from pathlib import Path
from typing import Callable, Optional

from config import STEAMCMD_DIR, PLATFORM

PROGRESS_REGEX = re.compile(
    r"Update state \(0x(?P<state>[0-9a-fA-F]+)\) downloading, progress:\s*(?P<percent>\d+\.\d+)\s*\((?P<current>\d+)\s*/\s*(?P<total>\d+)\)"
)

class SteamCMDService:
    def __init__(self, steamcmd_dir: Path = STEAMCMD_DIR):
        self.steamcmd_dir = steamcmd_dir
        self.steamcmd_dir.mkdir(parents=True, exist_ok=True)
        self.is_busy = False
        self.current_progress = 0.0
        self.current_status_text = "Idle"

    def get_executable_path(self) -> Path:
        if PLATFORM == "windows":
            return self.steamcmd_dir / "steamcmd.exe"
        else:
            return self.steamcmd_dir / "steamcmd.sh"

    async def ensure_installed(self, on_log: Optional[Callable[[str], None]] = None) -> bool:
        exe = self.get_executable_path()
        if exe.exists():
            return True

        if on_log:
            on_log(f"[SteamCMD] 二进制未找到，正在自动下载适配系统 ({PLATFORM}) 的 SteamCMD...")

        loop = asyncio.get_event_loop()
        success = await loop.run_in_executor(None, self._download_and_extract_sync, on_log)
        return success

    def _download_and_extract_sync(self, on_log: Optional[Callable[[str], None]] = None) -> bool:
        try:
            if PLATFORM == "windows":
                url = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"
                archive_path = self.steamcmd_dir / "steamcmd.zip"
                if on_log:
                    on_log(f"[SteamCMD] 正在从 {url} 下载...")
                urllib.request.urlretrieve(url, archive_path)
                with zipfile.ZipFile(archive_path, 'r') as zip_ref:
                    zip_ref.extractall(self.steamcmd_dir)
                archive_path.unlink(missing_ok=True)
            elif PLATFORM == "darwin":
                url = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_osx.tar.gz"
                archive_path = self.steamcmd_dir / "steamcmd_osx.tar.gz"
                if on_log:
                    on_log(f"[SteamCMD] 正在从 {url} 下载...")
                urllib.request.urlretrieve(url, archive_path)
                with tarfile.open(archive_path, "r:gz") as tar_ref:
                    tar_ref.extractall(self.steamcmd_dir)
                archive_path.unlink(missing_ok=True)
            else: # linux
                url = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"
                archive_path = self.steamcmd_dir / "steamcmd_linux.tar.gz"
                if on_log:
                    on_log(f"[SteamCMD] 正在从 {url} 下载...")
                urllib.request.urlretrieve(url, archive_path)
                with tarfile.open(archive_path, "r:gz") as tar_ref:
                    tar_ref.extractall(self.steamcmd_dir)
                archive_path.unlink(missing_ok=True)

            exe = self.get_executable_path()
            if exe.exists() and PLATFORM != "windows":
                exe.chmod(0o755)
            if on_log:
                on_log("[SteamCMD] 下载与解压完成。")
            return True
        except Exception as e:
            if on_log:
                on_log(f"[SteamCMD] 下载失败: {str(e)}")
            return False

    async def install_or_update(
        self,
        app_id: str,
        install_dir: Path,
        validate: bool = True,
        beta: Optional[str] = None,
        beta_password: Optional[str] = None,
        on_log: Optional[Callable[[str], None]] = None,
        on_progress: Optional[Callable[[float, str], None]] = None,
    ) -> bool:
        if self.is_busy:
            if on_log:
                on_log("[SteamCMD] 错误: 已有任务正在运行中，请等待完成。")
            return False

        self.is_busy = True
        self.current_progress = 0.0
        self.current_status_text = "Preparing SteamCMD"
        try:
            if not await self.ensure_installed(on_log):
                self.current_status_text = "SteamCMD download failed"
                return False
            exe = self.get_executable_path()
            if not exe.exists():
                if on_log:
                    on_log(f"[SteamCMD] 无法启动: 可执行文件 {exe} 不存在。")
                self.current_status_text = "SteamCMD executable missing"
                return False

            install_dir.mkdir(parents=True, exist_ok=True)
            self.current_status_text = "Starting SteamCMD"
            args = [str(exe), "+force_install_dir", str(install_dir.resolve()), "+login", "anonymous"]
            app_update_cmd = f"+app_update {app_id}"
            if beta:
                app_update_cmd += f" -beta {beta}"
                if beta_password:
                    app_update_cmd += f" -betapassword {beta_password}"
            if validate:
                app_update_cmd += " validate"
            args.extend([app_update_cmd, "+quit"])

            if on_log:
                on_log(f"[SteamCMD] 正在调度安装/更新 AppID: {app_id} -> {install_dir}")

            env = os.environ.copy()
            process = await asyncio.create_subprocess_exec(
                *args,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                cwd=str(self.steamcmd_dir),
                env=env,
            )
            if process.stdout:
                while True:
                    line_bytes = await process.stdout.readline()
                    if not line_bytes:
                        break
                    line = line_bytes.decode("utf-8", errors="replace").rstrip()
                    if not line:
                        continue
                    if on_log:
                        on_log(line)
                    match = PROGRESS_REGEX.search(line)
                    if match:
                        percent = float(match.group("percent"))
                        self.current_progress = percent
                        self.current_status_text = f"Downloading: {percent:.1f}%"
                        if on_progress:
                            on_progress(percent, self.current_status_text)
                    elif "Success! App" in line:
                        self.current_progress = 100.0
                        self.current_status_text = "Completed"
                        if on_progress:
                            on_progress(100.0, "Success")

            exit_code = await process.wait()
            if on_log:
                on_log(f"[SteamCMD] 任务退出，状态码: {exit_code}")
            self.current_status_text = "Completed" if exit_code == 0 else f"Failed (exit {exit_code})"
            return exit_code == 0
        except asyncio.CancelledError:
            self.current_status_text = "Cancelled"
            raise
        except Exception as e:
            if on_log:
                on_log(f"[SteamCMD] 运行异常: {str(e)}")
            self.current_status_text = f"Failed: {e}"
            return False
        finally:
            self.is_busy = False
