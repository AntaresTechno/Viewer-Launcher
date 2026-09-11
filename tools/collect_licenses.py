#!/usr/bin/env python3
"""Collect Python wheel licence metadata and notices without enforcing policy."""
from __future__ import annotations

import argparse
import json
import re
import shutil
from email.parser import BytesParser
from email.policy import default
from pathlib import Path


def licence_from(metadata) -> str:
    value = metadata.get("License-Expression") or metadata.get("License") or ""
    classifiers = metadata.get_all("Classifier", [])
    classified = [item.rsplit("::", 1)[-1].strip() for item in classifiers if "License ::" in item]
    return value.strip() or "; ".join(classified)


def safe_name(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]+", "_", value)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--site-packages", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--notices-dir", type=Path, required=True)
    args = parser.parse_args()
    entries = []
    args.notices_dir.mkdir(parents=True, exist_ok=True)

    for info in sorted(args.site_packages.glob("*.dist-info")):
        metadata_path = info / "METADATA"
        if not metadata_path.is_file():
            continue
        metadata = BytesParser(policy=default).parsebytes(metadata_path.read_bytes())
        name = metadata.get("Name", info.name).strip()
        version = metadata.get("Version", "unknown").strip()
        entries.append({"name": name, "version": version, "license": licence_from(metadata)})
        files = [
            path
            for pattern in ("LICENSE*", "COPYING*")
            for path in info.glob(pattern)
            if path.is_file()
        ]
        license_dir = info / "licenses"
        if license_dir.is_dir():
            files.extend(path for path in license_dir.rglob("*") if path.is_file())
        for source in files:
            destination = args.notices_dir / f"{safe_name(name)}-{safe_name(version)}-{safe_name(source.name)}"
            shutil.copyfile(source, destination)

    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(entries, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Collected licence metadata for {len(entries)} installed package(s).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
