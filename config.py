import os
import sys
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent

DATA_DIR = BASE_DIR / "data"
STEAMCMD_DIR = DATA_DIR / "steamcmd"
SERVERS_DIR = DATA_DIR / "servers"
TEMPLATES_DIR = BASE_DIR / "templates"
STATIC_DIR = BASE_DIR / "static"
LOGS_DIR = DATA_DIR / "logs"

API_HOST = os.getenv("GAMESERVER_HOST", "127.0.0.1")
API_PORT = int(os.getenv("GAMESERVER_PORT", "8769"))

if sys.platform.startswith("win"):
    PLATFORM = "windows"
elif sys.platform.startswith("linux"):
    PLATFORM = "linux"
elif sys.platform.startswith("darwin"):
    PLATFORM = "darwin"
else:
    PLATFORM = "unknown"

def ensure_directories():
    for d in [DATA_DIR, STEAMCMD_DIR, SERVERS_DIR, TEMPLATES_DIR, STATIC_DIR, LOGS_DIR]:
        d.mkdir(parents=True, exist_ok=True)
