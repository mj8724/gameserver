import asyncio
import os
import signal
import sys
import time
from collections import deque
from pathlib import Path
from typing import Callable, Deque, List, Optional, Set
import psutil

from config import PLATFORM

class ProcessSupervisor:
    def __init__(self, max_history_lines: int = 1000):
        self.max_history_lines = max_history_lines
        self.logs: Deque[str] = deque(maxlen=max_history_lines)
        self.listeners: Set[Callable[[str], None]] = set()

        self.process: Optional[asyncio.subprocess.Process] = None
        self.pid: Optional[int] = None
        self._read_task: Optional[asyncio.Task] = None

        self.status = "STOPPED"  # STOPPED, STARTING, RUNNING, CRASHED, STOPPING
        self.started_at: Optional[float] = None
        self.exit_code: Optional[int] = None

    def add_listener(self, listener: Callable[[str], None]):
        self.listeners.add(listener)

    def remove_listener(self, listener: Callable[[str], None]):
        self.listeners.discard(listener)

    def broadcast_line(self, line: str):
        self.logs.append(line)
        for listener in list(self.listeners):
            try:
                listener(line)
            except Exception:
                pass

    def get_logs(self, limit: int = 200) -> List[str]:
        return list(self.logs)[-limit:]

    async def start(self, cmd: List[str] | str, cwd: Path, env: Optional[dict] = None) -> bool:
        if self.is_running():
            self.broadcast_line("[Supervisor] 错误: 服务器进程已在运行中，请勿重复启动。")
            return False

        self.status = "STARTING"
        self.exit_code = None
        self.broadcast_line(f"[Supervisor] 准备启动进程，工作目录: {cwd}")
        if isinstance(cmd, list):
            safe_cmd = []
            redact_next = False
            for part in cmd:
                if redact_next:
                    safe_cmd.append("[REDACTED]")
                    redact_next = False
                elif str(part).lower() == "-adminpassword":
                    safe_cmd.append(str(part))
                    redact_next = True
                elif str(part).lower().startswith("-adminpassword="):
                    safe_cmd.append("-adminpassword=[REDACTED]")
                else:
                    safe_cmd.append(str(part))
            self.broadcast_line(f"[Supervisor] 执行命令: {' '.join(safe_cmd)}")
        else:
            self.broadcast_line(f"[Supervisor] 执行命令: {cmd}")

        full_env = os.environ.copy()
        if env:
            full_env.update(env)

        try:
            if isinstance(cmd, str):
                self.process = await asyncio.create_subprocess_shell(
                    cmd,
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.STDOUT,
                    cwd=str(cwd.resolve()),
                    env=full_env,
                )
            else:
                self.process = await asyncio.create_subprocess_exec(
                    *cmd,
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.STDOUT,
                    cwd=str(cwd.resolve()),
                    env=full_env,
                )

            self.pid = self.process.pid
            self.started_at = time.time()
            self.status = "RUNNING"
            self.broadcast_line(f"[Supervisor] 进程已创建，PID: {self.pid}")

            process = self.process
            self._read_task = asyncio.create_task(self._stream_reader())
            await asyncio.sleep(0)
            if process.returncode is not None:
                await self._read_task
                return False
            return True
        except Exception as e:
            self.status = "CRASHED"
            self.broadcast_line(f"[Supervisor] 启动失败: {str(e)}")
            return False

    async def _stream_reader(self):
        process = self.process
        if not process or not process.stdout:
            return

        try:
            while True:
                line_bytes = await process.stdout.readline()
                if not line_bytes:
                    break
                line = line_bytes.decode("utf-8", errors="replace").rstrip()
                if line:
                    self.broadcast_line(line)
        except Exception as e:
            self.broadcast_line(f"[Supervisor] 日志流异常: {str(e)}")
        finally:
            self.exit_code = await process.wait()
            self.broadcast_line(f"[Supervisor] 进程已退出，退出码: {self.exit_code}")
            self.status = "CRASHED" if self.exit_code != 0 and self.status != "STOPPING" else "STOPPED"
            self.pid = None
            if self.process is process:
                self.process = None

    async def send_input(self, text: str) -> bool:
        if not self.is_running() or not self.process or not self.process.stdin:
            return False

        try:
            cmd = text.strip() + "\n"
            self.process.stdin.write(cmd.encode("utf-8"))
            await self.process.stdin.drain()
            self.broadcast_line(f"> {text.strip()}")
            return True
        except (BrokenPipeError, ConnectionResetError, RuntimeError) as e:
            self.broadcast_line(f"[Supervisor] 发送输入失败: {str(e)}")
            return False

    async def stop(self, stop_command: str = "quit", timeout: int = 25) -> bool:
        process = self.process
        if not process or process.returncode is not None:
            if process:
                await process.wait()
            self.status = "STOPPED"
            return True

        self.status = "STOPPING"
        self.broadcast_line(f"[Supervisor] 正在尝试通过指令 '{stop_command}' 优雅停止服务器...")
        await self.send_input(stop_command)
        try:
            await asyncio.wait_for(process.wait(), timeout=max(0, timeout))
            if self._read_task:
                await self._read_task
            self.status = "STOPPED"
            self.broadcast_line("[Supervisor] 服务器已安全退出。")
            return True
        except asyncio.TimeoutError:
            self.broadcast_line("[Supervisor] 优雅停机超时，正在执行强制终止 (Force Kill)...")
            if not self.kill():
                return False
            await process.wait()
            if self._read_task:
                await self._read_task
            self.status = "STOPPED"
            return True

    def kill(self) -> bool:
        if not self.process or self.process.returncode is not None:
            self.status = "STOPPED"
            return True

        try:
            # Terminate children first via psutil
            if self.pid:
                try:
                    parent = psutil.Process(self.pid)
                    for child in parent.children(recursive=True):
                        child.kill()
                    parent.kill()
                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    pass

            self.process.kill()
            self.status = "STOPPED"
            self.broadcast_line("[Supervisor] 进程已被强制终止。")
            return True
        except Exception as e:
            self.broadcast_line(f"[Supervisor] 强制终止失败: {str(e)}")
            return False

    def is_running(self) -> bool:
        if self.process is None:
            return False
        return self.process.returncode is None

    def get_metrics(self) -> dict:
        if not self.is_running() or not self.pid:
            return {
                "running": False,
                "status": self.status,
                "cpu_percent": 0.0,
                "memory_mb": 0.0,
                "uptime_seconds": 0,
            }

        cpu_total = 0.0
        memory_total = 0.0
        try:
            parent = psutil.Process(self.pid)
            cpu_total += parent.cpu_percent(interval=None)
            memory_total += parent.memory_info().rss

            for child in parent.children(recursive=True):
                try:
                    cpu_total += child.cpu_percent(interval=None)
                    memory_total += child.memory_info().rss
                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    continue
        except (psutil.NoSuchProcess, psutil.AccessDenied):
            pass

        uptime = int(time.time() - (self.started_at or time.time()))
        return {
            "running": True,
            "status": self.status,
            "pid": self.pid,
            "cpu_percent": round(cpu_total, 1),
            "memory_mb": round(memory_total / (1024 * 1024), 1),
            "uptime_seconds": uptime,
        }
