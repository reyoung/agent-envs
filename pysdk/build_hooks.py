from __future__ import annotations

from pathlib import Path
import subprocess

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


def _find_buf_root(start: Path) -> Path:
    for candidate in (start, *start.parents):
        if (candidate / "buf.yaml").is_file():
            return candidate
    raise RuntimeError("buf.yaml not found; cannot run buf generate")


class CustomBuildHook(BuildHookInterface):
    def initialize(self, version: str, build_data: dict) -> None:
        buf_root = _find_buf_root(Path(self.root))
        subprocess.run(["buf", "generate"], cwd=buf_root, check=True)
