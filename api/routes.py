import asyncio
import hashlib
import hmac
import json
import os
import re
import secrets
import time
from typing import Any, Dict, Optional

from fastapi import FastAPI, HTTPException, Request, WebSocket, WebSocketDisconnect
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, ConfigDict, Field, StrictInt

from config import STATIC_DIR
from core.instance_manager import InstanceManager
from core.process_supervisor import ProcessSupervisor
from core.steamcmd_service import SteamCMDService
from core.template_manager import TemplateManager

SESSION_COOKIE = "gameserver_session"
SESSION_TTL_SECONDS = 12 * 60 * 60
SERVER_NAME_PATTERN = re.compile(r"^[A-Za-z0-9_-]{1,64}$")


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class LoginRequest(StrictModel):
    password: str


class CommandRequest(StrictModel):
    command: str = Field(min_length=1, max_length=4096)


class ConfigUpdateRequest(StrictModel):
    variables: Dict[str, Any] = Field(default_factory=dict)
    ports: Optional[Dict[str, StrictInt]] = None


class ModAddRequest(StrictModel):
    workshop_id: str = Field(min_length=1, max_length=32)
    mod_name: Optional[str] = Field(default=None, max_length=128)


class RenewalRequest(StrictModel):
    months: int = Field(default=1, ge=1, le=24)


def _session_signature(timestamp: str, secret: bytes) -> str:
    return hmac.new(secret, timestamp.encode(), hashlib.sha256).hexdigest()


def _valid_session(request: Request, session_secret: bytes) -> bool:
    token = request.cookies.get(SESSION_COOKIE, "")
    try:
        timestamp, signature = token.split(".", 1)
        issued_at = int(timestamp)
    except (ValueError, TypeError):
        return False
    if issued_at > int(time.time()) or time.time() - issued_at > SESSION_TTL_SECONDS:
        return False
    return hmac.compare_digest(signature, _session_signature(timestamp, session_secret))


def _redacted_variables(variables: Dict[str, Any]) -> Dict[str, Any]:
    return {
        key: ("" if key == "SERVER_PASSWORD" else None if value else "")
        if key in {"SERVER_PASSWORD", "ADMIN_PASSWORD"}
        else value
        for key, value in variables.items()
    }


def _validate_config(template: dict, variables: Dict[str, Any], ports: Dict[str, int]) -> None:
    declared_variables = {item["key"]: item for item in template.get("variables", [])}
    declared_ports = {item["key"] for item in template.get("ports", [])}

    for key, value in variables.items():
        definition = declared_variables.get(key)
        if definition is None or not definition.get("user_editable", False):
            raise HTTPException(status_code=422, detail=f"不允许修改配置项: {key}")
        field_type = definition.get("type")
        if field_type == "string":
            if not isinstance(value, str):
                raise HTTPException(status_code=422, detail=f"{key} 必须是文本")
            if key == "SERVER_NAME" and not SERVER_NAME_PATTERN.fullmatch(value):
                raise HTTPException(status_code=422, detail="服务器名称仅允许 1-64 位字母、数字、下划线和连字符")
            if len(value) > 256:
                raise HTTPException(status_code=422, detail=f"{key} 过长")
        elif field_type == "password":
            if not isinstance(value, str) or len(value) > 256:
                raise HTTPException(status_code=422, detail=f"{key} 必须是 256 字符以内的文本")
            if definition.get("required") and not value:
                raise HTTPException(status_code=422, detail=f"{key} 不能为空")
        elif field_type == "number":
            if isinstance(value, bool) or not isinstance(value, int):
                raise HTTPException(status_code=422, detail=f"{key} 必须是整数")
            validation = definition.get("validation", {})
            if value < validation.get("min", value) or value > validation.get("max", value):
                raise HTTPException(status_code=422, detail=f"{key} 超出允许范围")
        elif field_type == "boolean" and not isinstance(value, bool):
            raise HTTPException(status_code=422, detail=f"{key} 必须是布尔值")

    for key, value in ports.items():
        if key not in declared_ports:
            raise HTTPException(status_code=422, detail=f"不允许修改端口: {key}")
        if not 1 <= value <= 65535:
            raise HTTPException(status_code=422, detail=f"{key} 必须在 1-65535 之间")


def create_app(
    instance_manager: Optional[InstanceManager] = None,
    template_manager: Optional[TemplateManager] = None,
    steamcmd_service: Optional[SteamCMDService] = None,
    supervisor: Optional[ProcessSupervisor] = None,
    admin_password: Optional[str] = None,
) -> FastAPI:
    templates = template_manager or TemplateManager()
    steamcmd = steamcmd_service or SteamCMDService()
    process_supervisor = supervisor or ProcessSupervisor()
    instance = instance_manager or InstanceManager(
        instance_id="pz_01",
        template_manager=templates,
        steamcmd_service=steamcmd,
        supervisor=process_supervisor,
    )
    configured_password = admin_password if admin_password is not None else os.getenv("GAMESERVER_ADMIN_PASSWORD", "")
    session_secret = secrets.token_bytes(32)
    install_state: Dict[str, Any] = {"status": "IDLE", "progress": 0.0, "message": "尚未运行", "error": None}
    install_task: Optional[asyncio.Task] = None

    app = FastAPI(title="Steam Game Server Web Control Panel", version="1.0.0")
    app.state.instance_manager = instance
    app.state.template_manager = templates
    app.state.steamcmd_service = steamcmd
    app.state.supervisor = process_supervisor

    @app.get("/api/auth/status")
    async def auth_status(request: Request):
        return {"authenticated": bool(configured_password and _valid_session(request, session_secret))}

    @app.post("/api/auth/login")
    async def login(req: LoginRequest):
        if not configured_password:
            raise HTTPException(status_code=503, detail="尚未配置 GAMESERVER_ADMIN_PASSWORD")
        if not hmac.compare_digest(req.password.encode(), configured_password.encode()):
            raise HTTPException(status_code=401, detail="管理员密码错误")
        timestamp = str(int(time.time()))
        response_token = f"{timestamp}.{_session_signature(timestamp, session_secret)}"
        from fastapi.responses import JSONResponse
        response = JSONResponse({"authenticated": True})
        response.set_cookie(
            SESSION_COOKIE,
            response_token,
            httponly=True,
            secure=os.getenv("GAMESERVER_COOKIE_SECURE", "0") == "1",
            samesite="strict",
            max_age=SESSION_TTL_SECONDS,
            path="/",
        )
        return response

    @app.post("/api/auth/logout")
    async def logout():
        from fastapi.responses import JSONResponse
        response = JSONResponse({"authenticated": False})
        response.delete_cookie(SESSION_COOKIE, path="/")
        return response

    async def require_session(request: Request):
        if not configured_password:
            raise HTTPException(status_code=503, detail="尚未配置 GAMESERVER_ADMIN_PASSWORD")
        origin = request.headers.get("origin")
        if origin:
            from urllib.parse import urlsplit
            if urlsplit(origin).netloc != request.headers.get("host"):
                raise HTTPException(status_code=403, detail="跨站请求已拒绝")
        if not _valid_session(request, session_secret):
            raise HTTPException(status_code=401, detail="请先登录")

    @app.get("/api/status")
    async def get_status(request: Request):
        await require_session(request)
        result = instance.get_summary()
        result["variables"] = _redacted_variables(result.get("variables", {}))
        result["install_task"] = dict(install_state)
        return result

    @app.get("/api/templates")
    async def list_templates(request: Request):
        await require_session(request)
        return templates.list_templates()

    @app.post("/api/server/install", status_code=202)
    async def install_server(request: Request):
        nonlocal install_task
        await require_session(request)
        if install_task and not install_task.done():
            raise HTTPException(status_code=409, detail="已有安装任务在运行中")
        if process_supervisor.is_running():
            raise HTTPException(status_code=409, detail="服务器运行中，请先关机后再更新")

        install_state.update(status="INSTALLING", progress=0.0, message="准备安装", error=None)

        def on_progress(percent: float, message: str):
            install_state.update(progress=percent, message=message)

        async def run_install():
            try:
                success = await instance.install(validate=True, on_progress=on_progress)
                install_state.update(
                    status="COMPLETED" if success else "FAILED",
                    progress=100.0 if success else install_state["progress"],
                    message="安装完成" if success else "安装失败，请查看日志",
                    error=None if success else "SteamCMD 安装或更新未成功",
                )
            except Exception as exc:
                install_state.update(status="FAILED", message="安装失败", error=str(exc))
            finally:
                steamcmd.is_busy = False

        install_task = asyncio.create_task(run_install())
        return {"message": "安装/更新任务已启动", "status": install_state["status"]}

    @app.get("/api/server/install")
    async def get_install_status(request: Request):
        await require_session(request)
        return dict(install_state)

    @app.post("/api/server/start")
    async def start_server(request: Request):
        await require_session(request)
        if process_supervisor.is_running():
            return {"message": "服务器已在运行中", "running": True}
        if steamcmd.is_busy or (install_task and not install_task.done()):
            raise HTTPException(status_code=409, detail="正在安装中，请稍候启动")
        if not instance.is_installed():
            raise HTTPException(status_code=400, detail="游戏服务端未安装，请先安装服务端")
        if not await instance.start():
            raise HTTPException(status_code=500, detail="服务器启动失败，请检查控制台输出")
        return {"message": "启动指令已执行", "running": True}

    @app.post("/api/server/stop")
    async def stop_server(request: Request):
        await require_session(request)
        success = await instance.stop()
        if not success:
            raise HTTPException(status_code=500, detail="服务器停止失败")
        return {"message": "服务器已停止", "success": True}

    @app.post("/api/server/restart")
    async def restart_server(request: Request):
        await require_session(request)
        if process_supervisor.is_running() and not await instance.stop():
            raise HTTPException(status_code=500, detail="停止服务器失败，已取消重启")
        if not instance.is_installed():
            raise HTTPException(status_code=400, detail="游戏服务端未安装")
        if not await instance.start():
            raise HTTPException(status_code=500, detail="服务器重启失败，请检查控制台输出")
        return {"message": "服务器已重启", "success": True}

    @app.post("/api/server/kill")
    async def kill_server(request: Request):
        await require_session(request)
        success = instance.kill()
        return {"message": "强制终止指令已执行", "success": success}

    @app.post("/api/server/command")
    async def send_command(req: CommandRequest, request: Request):
        await require_session(request)
        if not process_supervisor.is_running():
            raise HTTPException(status_code=400, detail="服务器未运行，无法发送控制台指令")
        success = await process_supervisor.send_input(req.command)
        return {"success": success}

    @app.get("/api/server/logs")
    async def get_logs(request: Request, limit: int = 150):
        await require_session(request)
        return {"logs": process_supervisor.get_logs(max(1, min(limit, 1000)))}

    @app.get("/api/server/config")
    async def get_config(request: Request):
        await require_session(request)
        current = instance.state["variables"]
        definitions = templates.get_template(instance.state["template_id"]).get("variables", [])
        safe_fields = []
        for item in definitions:
            if not item.get("user_editable", False):
                continue
            field = {**item, "value": None if item.get("type") == "password" else current.get(item["key"], item.get("default"))}
            if item.get("type") == "password":
                field.pop("default", None)
            safe_fields.append(field)
        return {
            "variables": _redacted_variables(current),
            "ports": instance.state["ports"],
            "fields": safe_fields,
            "template": {"id": instance.state["template_id"], "name": templates.get_template(instance.state["template_id"])["metadata"]["name"]},
        }

    @app.post("/api/server/config")
    async def update_config(req: ConfigUpdateRequest, request: Request):
        await require_session(request)
        template = templates.get_template(instance.state["template_id"])
        _validate_config(template, req.variables, req.ports or {})
        if req.variables:
            instance.update_variables(req.variables)
        if req.ports:
            instance.update_ports(req.ports)
        safe_state = dict(instance.state)
        safe_state["variables"] = _redacted_variables(instance.state["variables"])
        return {"message": "配置已保存并同步", "state": safe_state}

    @app.post("/api/server/mods")
    async def add_mod(req: ModAddRequest, request: Request):
        await require_session(request)
        if not req.workshop_id.isdigit():
            raise HTTPException(status_code=422, detail="Workshop ID 必须为数字")
        instance.add_workshop_mod(req.workshop_id, req.mod_name)
        return {"message": "模组已登记（尚未下载）", "mods": instance.state["mods"]}

    @app.delete("/api/server/mods/{workshop_id}")
    async def remove_mod(workshop_id: str, request: Request):
        await require_session(request)
        instance.remove_workshop_mod(workshop_id)
        return {"message": "模组已移除", "mods": instance.state["mods"]}

    @app.post("/api/server/renew")
    async def renew_subscription(req: RenewalRequest, request: Request):
        await require_session(request)
        current_days = instance.state["billing"].get("expire_days_left", 30)
        instance.state["billing"]["expire_days_left"] = current_days + (req.months * 30)
        instance.state["billing"]["status"] = "ACTIVE"
        instance.save_state()
        process_supervisor.broadcast_line(f"[Billing] 模拟续费成功: 增加 {req.months} 个月有效服务期！")
        return {"message": f"模拟续费成功，剩余天数: {instance.state['billing']['expire_days_left']} 天", "billing": instance.state["billing"]}

    @app.websocket("/ws/console")
    async def websocket_console(websocket: WebSocket):
        if not configured_password or not _valid_session(websocket, session_secret):
            await websocket.close(code=1008)
            return
        origin = websocket.headers.get("origin")
        if origin:
            from urllib.parse import urlsplit
            if urlsplit(origin).netloc != websocket.headers.get("host"):
                await websocket.close(code=1008)
                return
        await websocket.accept()
        for line in process_supervisor.get_logs(limit=100):
            await websocket.send_text(json.dumps({"type": "log", "data": line}))
        queue: asyncio.Queue = asyncio.Queue()

        def on_line(text: str):
            queue.put_nowait(text)

        process_supervisor.add_listener(on_line)

        async def sender_task():
            while True:
                line = await queue.get()
                await websocket.send_text(json.dumps({"type": "log", "data": line}))

        task = asyncio.create_task(sender_task())
        try:
            while True:
                raw_msg = await websocket.receive_text()
                try:
                    msg = json.loads(raw_msg)
                except json.JSONDecodeError:
                    msg = {"type": "input", "data": raw_msg}
                if msg.get("type") == "input" and msg.get("data"):
                    await process_supervisor.send_input(str(msg["data"]))
                elif msg.get("type") == "ping":
                    await websocket.send_text(json.dumps({"type": "pong"}))
        except WebSocketDisconnect:
            pass
        finally:
            task.cancel()
            process_supervisor.remove_listener(on_line)

    STATIC_DIR.mkdir(parents=True, exist_ok=True)
    app.mount("/static", StaticFiles(directory=str(STATIC_DIR)), name="static")

    @app.get("/")
    async def root():
        index_file = STATIC_DIR / "index.html"
        if index_file.exists():
            return FileResponse(index_file)
        return {"message": "Gameserver Backend Running"}

    return app


app = create_app()
