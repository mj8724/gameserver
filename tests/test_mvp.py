import asyncio
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from starlette.websockets import WebSocketDisconnect

from api.routes import create_app
from core.instance_manager import InstanceManager
from core.process_supervisor import ProcessSupervisor
from core.steamcmd_service import SteamCMDService
from core.template_manager import TemplateManager
from core.zomboid_config import ZomboidConfigHandler


class FakeInstance:
    def __init__(self, tmp_path, template_manager, steamcmd, supervisor):
        self.state = {
            "instance_id": "pz_test",
            "name": "Test Server",
            "template_id": "project_zomboid",
            "variables": {
                "SERVER_NAME": "servertest",
                "SERVER_PASSWORD": "join-secret",
                "ADMIN_PASSWORD": "admin-secret",
                "MAX_PLAYERS": 16,
                "PVP_ENABLED": True,
                "PUBLIC_SERVER": True,
                "OPEN_REGISTRATION": True,
                "PAUSE_EMPTY": True,
            },
            "ports": {"SERVER_PORT": 16261, "DIRECT_PORT": 16262},
            "mods": {"workshop_ids": [], "mod_names": []},
            "billing": {"expire_days_left": 28, "status": "ACTIVE"},
        }
        self.cache_dir = tmp_path / "Zomboid"
        self.cache_dir.mkdir()
        self.template_manager = template_manager
        self.steamcmd_service = steamcmd
        self.supervisor = supervisor

    def get_summary(self):
        return {
            "instance_id": self.state["instance_id"],
            "status": "STOPPED",
            "running": False,
            "is_installed": False,
            "variables": self.state["variables"],
            "ports": self.state["ports"],
        }

    def is_installed(self):
        return False

    async def install(self, **kwargs):
        return False

    async def start(self):
        return True

    async def stop(self):
        return True

    def kill(self):
        return True

    def update_variables(self, values):
        self.state["variables"].update(values)

    def update_ports(self, values):
        self.state["ports"].update(values)

    def add_workshop_mod(self, workshop_id, mod_name=None):
        self.state["mods"]["workshop_ids"].append(workshop_id)
        if mod_name:
            self.state["mods"]["mod_names"].append(mod_name)

    def remove_workshop_mod(self, workshop_id):
        if workshop_id in self.state["mods"]["workshop_ids"]:
            self.state["mods"]["workshop_ids"].remove(workshop_id)

    def save_state(self):
        pass


def make_client(tmp_path, password="test-admin"):
    template_manager = TemplateManager()
    steamcmd = SteamCMDService(tmp_path / "steamcmd")
    supervisor = ProcessSupervisor()
    instance = FakeInstance(tmp_path, template_manager, steamcmd, supervisor)
    app = create_app(instance, template_manager, steamcmd, supervisor, password)
    return TestClient(app), instance, steamcmd, supervisor


def login(client):
    response = client.post("/api/auth/login", json={"password": "test-admin"})
    assert response.status_code == 200


def test_management_routes_require_authentication(tmp_path):
    client, _, _, _ = make_client(tmp_path)

    assert client.get("/api/status").status_code == 401
    assert client.get("/api/server/config").status_code == 401
    assert client.post("/api/server/start").status_code == 401
    with pytest.raises(WebSocketDisconnect):
        with client.websocket_connect("/ws/console"):
            pass

    login(client)
    assert client.get("/api/status").status_code == 200
    with client.websocket_connect("/ws/console"):
        pass


def test_status_and_config_never_expose_passwords(tmp_path):
    client, _, _, _ = make_client(tmp_path)
    login(client)

    status = client.get("/api/status").text
    config = client.get("/api/server/config").text
    assert "admin-secret" not in status + config
    assert "join-secret" not in status + config
    assert "Admin123456" not in config


def test_config_rejects_path_traversal_unknown_fields_and_invalid_values(tmp_path):
    client, _, _, _ = make_client(tmp_path)
    login(client)

    traversal = client.post("/api/server/config", json={"variables": {"SERVER_NAME": "../outside"}})
    unknown = client.post("/api/server/config", json={"variables": {"NOT_A_FIELD": "x"}})
    bad_players = client.post("/api/server/config", json={"variables": {"MAX_PLAYERS": 100}})
    bad_port = client.post("/api/server/config", json={"ports": {"SERVER_PORT": 70000}})

    assert traversal.status_code == 422
    assert unknown.status_code == 422
    assert bad_players.status_code == 422
    assert bad_port.status_code == 422


def test_config_password_is_unchanged_when_omitted(tmp_path):
    client, instance, _, _ = make_client(tmp_path)
    login(client)

    response = client.post("/api/server/config", json={"variables": {"MAX_PLAYERS": 24}})

    assert response.status_code == 200
    assert instance.state["variables"]["ADMIN_PASSWORD"] == "admin-secret"
    assert instance.state["variables"]["SERVER_PASSWORD"] == "join-secret"


def test_ini_writer_clears_empty_mod_lists_and_rejects_unsafe_names(tmp_path):
    ini_path = tmp_path / "servertest.ini"
    ini_path.write_text("WorkshopItems=123;456\nMods=modA;modB\n", encoding="utf-8")

    ZomboidConfigHandler.write_config(ini_path, {"WorkshopItems": "", "Mods": ""})

    assert "WorkshopItems=\n" in ini_path.read_text(encoding="utf-8")
    assert "Mods=\n" in ini_path.read_text(encoding="utf-8")
    with pytest.raises(ValueError):
        ZomboidConfigHandler.get_ini_path(tmp_path, "../escape")


def test_instance_state_load_merges_nested_defaults(tmp_path, monkeypatch):
    from core import instance_manager as module

    monkeypatch.setattr(module, "SERVERS_DIR", tmp_path / "servers")
    manager = InstanceManager("test", TemplateManager(), SteamCMDService(tmp_path / "steamcmd"), ProcessSupervisor())
    manager.state_file.write_text('{"variables": {"SERVER_NAME": "safe_name"}}', encoding="utf-8")

    manager.load_state()

    assert manager.state["variables"]["SERVER_NAME"] == "safe_name"
    assert len(manager.state["variables"]["ADMIN_PASSWORD"]) >= 32
    assert manager.state["ports"]["SERVER_PORT"] == 16261


def test_steamcmd_initialization_failure_clears_busy_flag(tmp_path, monkeypatch):
    service = SteamCMDService(tmp_path / "steamcmd")
    async def fail_install(_on_log=None):
        return False
    monkeypatch.setattr(service, "ensure_installed", fail_install)

    result = asyncio.run(service.install_or_update("380870", tmp_path / "server"))

    assert result is False
    assert service.is_busy is False
    assert service.current_status_text == "SteamCMD download failed"
