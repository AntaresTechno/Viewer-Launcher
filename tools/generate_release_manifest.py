#!/usr/bin/env python3
"""Generate the machine-readable manifest and prerelease asset table."""

from __future__ import annotations

import argparse
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path


ASSETS = {
    "viewer-web.tar.gz": ("web", "all", "web", "Viewer 前端静态文件"),
    "viewer-backend.tar.gz": ("backend", "all", "backend", "Viewer FastAPI 后端源码"),
    "viewer-runtime-windows-amd64.tar.gz": ("runtime", "windows-amd64", "lib", "Windows x64 Python 与依赖"),
    "viewer-runtime-linux-amd64.tar.gz": ("runtime", "linux-amd64", "lib", "Linux x64 Python 与依赖"),
    "viewer-runtime-linux-arm64.tar.gz": ("runtime", "linux-arm64", "lib", "Linux ARM64 Python 与依赖"),
    "viewer-launchers-windows-amd64.zip": ("launchers", "windows-amd64", "", "Windows x64 桌面版与 CLI 启动器"),
    "viewer-launchers-linux-amd64.tar.gz": ("launchers", "linux-amd64", "", "Linux x64 桌面版与 CLI 启动器"),
    "viewer-launchers-linux-arm64.tar.gz": ("launchers", "linux-arm64", "", "Linux ARM64 桌面版与 CLI 启动器"),
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--assets", type=Path, required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--upstream-commit", required=True)
    parser.add_argument("--tag", default="preview")
    parser.add_argument("--notes", type=Path, required=True)
    args = parser.parse_args()

    present = {item.name for item in args.assets.iterdir() if item.is_file()}
    missing = set(ASSETS) - present
    unexpected = present - set(ASSETS)
    if missing or unexpected:
        raise SystemExit(f"release assets differ from specification; missing={sorted(missing)}, unexpected={sorted(unexpected)}")

    assets = []
    for name, (kind, platform, install_dir, description) in ASSETS.items():
        file = args.assets / name
        assets.append(
            {
                "name": name,
                "kind": kind,
                "platform": platform,
                **({"install_dir": install_dir} if install_dir else {}),
                "description": description,
                "sha256": sha256(file),
                "size": file.stat().st_size,
            }
        )

    manifest = {
        "schema_version": 1,
        "release_tag": args.tag,
        "source_repository": args.repository,
        "source_commit": args.source_commit,
        "upstream_repository": "AntaresTechno/Viewer",
        "upstream_commit": args.upstream_commit,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "assets": assets,
    }
    manifest_path = args.assets / "release-manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    rows = assets + [
        {
            "name": manifest_path.name,
            "description": "启动器使用的机器可读发布清单",
            "platform": "all",
            "size": manifest_path.stat().st_size,
            "sha256": sha256(manifest_path),
        }
    ]
    lines = [
        "# Viewer Launcher Preview",
        "",
        f"- 启动器提交：`{args.source_commit}`",
        f"- Viewer 提交：`{args.upstream_commit}`",
        "- 此版本为自动更新的预发布版本。相同文件名会在下一次构建时替换。",
        "",
        "| 文件 | 作用 | 平台 | 大小（bytes） | SHA-256 |",
        "| --- | --- | --- | ---: | --- |",
    ]
    lines.extend(
        f"| `{row['name']}` | {row['description']} | `{row['platform']}` | {row['size']} | `{row['sha256']}` |"
        for row in rows
    )
    args.notes.write_text("\n".join(lines) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
