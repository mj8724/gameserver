import argparse
import os
import sys
import webbrowser
from pathlib import Path
import uvicorn

from config import API_HOST, API_PORT, ensure_directories

def main():
    parser = argparse.ArgumentParser(description="Steam Game Server Web Control Panel")
    parser.add_argument("--host", default=API_HOST, help="Binding host address")
    parser.add_argument("--port", type=int, default=API_PORT, help="Binding port number")
    parser.add_argument("--browser", action="store_true", help="Automatically open browser")
    args = parser.parse_args()

    ensure_directories()

    url = f"http://{'127.0.0.1' if args.host == '0.0.0.0' else args.host}:{args.port}"
    print("=" * 65)
    print("  🚀 Steam 游戏服务器 Web 控制面 (Project Zomboid Dedicated)")
    print(f"  👉 控制台访问地址: {url}")
    print("=" * 65)

    if args.browser:
        webbrowser.open(url)

    uvicorn.run("api.routes:app", host=args.host, port=args.port, log_level="info")

if __name__ == "__main__":
    main()
