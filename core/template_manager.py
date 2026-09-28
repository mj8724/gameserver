import os
from pathlib import Path
from typing import Dict, List, Optional
import yaml

from config import TEMPLATES_DIR

class TemplateManager:
    def __init__(self, templates_dir: Path = TEMPLATES_DIR):
        self.templates_dir = templates_dir
        self.templates: Dict[str, dict] = {}
        self.load_all()

    def load_all(self):
        self.templates_dir.mkdir(parents=True, exist_ok=True)
        self.templates.clear()
        for f in self.templates_dir.glob("*.yaml"):
            try:
                with open(f, "r", encoding="utf-8") as stream:
                    data = yaml.safe_load(stream)
                    template_id = data.get("metadata", {}).get("id", f.stem)
                    self.templates[template_id] = data
            except Exception as e:
                print(f"[TemplateManager] 警告: 加载模板 {f.name} 失败: {e}")

    def get_template(self, template_id: str) -> Optional[dict]:
        if template_id not in self.templates:
            self.load_all()
        return self.templates.get(template_id)

    def list_templates(self) -> List[dict]:
        self.load_all()
        result = []
        for tid, t in self.templates.items():
            meta = t.get("metadata", {})
            steam = t.get("steam", {})
            result.append({
                "id": tid,
                "name": meta.get("name", tid),
                "category": meta.get("category", "General"),
                "icon": meta.get("icon", ""),
                "author": meta.get("author", "Official"),
                "version": meta.get("version", "1.0.0"),
                "description": meta.get("description", ""),
                "supported_os": meta.get("supported_os", ["windows", "linux"]),
                "app_id": steam.get("app_id", ""),
            })
        return result
