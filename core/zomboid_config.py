import os
from pathlib import Path
from typing import Dict, Any

class ZomboidConfigHandler:
    DEFAULT_SETTINGS = {
        "Public": "true",
        "Password": "",
        "Open": "true",
        "PVP": "true",
        "PauseEmpty": "true",
        "DefaultPort": "16261",
        "UDPPort": "16262",
        "MaxPlayers": "16",
        "Mods": "",
        "WorkshopItems": "",
        "PingLimit": "400",
        "AutoCreateUserInWhiteList": "true",
        "DisplayUserName": "true",
        "SpawnItems": "",
        "RCONPort": "27015",
        "RCONPassword": "",
    }

    @staticmethod
    def get_ini_path(cache_dir: Path, server_name: str) -> Path:
        if not server_name or any(char not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-" for char in server_name) or len(server_name) > 64:
            raise ValueError("Invalid server name")
        server_dir = cache_dir / "Server"
        server_dir.mkdir(parents=True, exist_ok=True)
        return server_dir / f"{server_name}.ini"

    @classmethod
    def read_config(cls, ini_path: Path) -> Dict[str, str]:
        if not ini_path.exists():
            return cls.DEFAULT_SETTINGS.copy()

        config = {}
        with open(ini_path, "r", encoding="utf-8", errors="replace") as f:
            for line in f:
                line_str = line.strip()
                if not line_str or line_str.startswith("#"):
                    continue
                if "=" in line_str:
                    key, val = line_str.split("=", 1)
                    config[key.strip()] = val.strip()
        return config

    @classmethod
    def write_config(cls, ini_path: Path, updates: Dict[str, Any]):
        ini_path.parent.mkdir(parents=True, exist_ok=True)

        existing_lines = []
        if ini_path.exists():
            with open(ini_path, "r", encoding="utf-8", errors="replace") as f:
                existing_lines = f.readlines()

        existing_keys = set()
        new_lines = []

        # Update existing lines
        for line in existing_lines:
            stripped = line.strip()
            if not stripped or stripped.startswith("#"):
                new_lines.append(line)
                continue
            if "=" in stripped:
                k, _ = stripped.split("=", 1)
                k = k.strip()
                existing_keys.add(k)
                if k in updates:
                    val_str = str(updates[k])
                    if isinstance(updates[k], bool):
                        val_str = "true" if updates[k] else "false"
                    new_lines.append(f"{k}={val_str}\n")
                else:
                    new_lines.append(line)
            else:
                new_lines.append(line)

        # Append missing keys
        for k, v in updates.items():
            if k not in existing_keys:
                val_str = str(v)
                if isinstance(v, bool):
                    val_str = "true" if v else "false"
                new_lines.append(f"{k}={val_str}\n")

        with open(ini_path, "w", encoding="utf-8") as f:
            f.writelines(new_lines)
