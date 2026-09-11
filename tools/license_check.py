#!/usr/bin/env python3
"""Fail closed on Python package licences and preserve discovered notices."""
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
    parser.add_argument("--policy", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--notices-dir", type=Path, required=True)
    args = parser.parse_args()
    policy = json.loads(args.policy.read_text(encoding="utf-8"))
    allowed = tuple(item.lower() for item in policy["allowed_markers"])
    denied = tuple(item.lower() for item in policy["denied_markers"])
    exceptions = {key.lower(): value for key, value in policy.get("approved_packages", {}).items()}
    entries, failures = [], []
    args.notices_dir.mkdir(parents=True, exist_ok=True)

    for info in sorted(args.site_packages.glob("*.dist-info")):
        metadata_path = info / "METADATA"
        if not metadata_path.exists():
            continue
        metadata = BytesParser(policy=default).parsebytes(metadata_path.read_bytes())
        name = metadata.get("Name", info.name).strip()
        version = metadata.get("Version", "unknown").strip()
        licence = licence_from(metadata)
        normalized, package_key = licence.lower(), name.lower()
        permitted = (
            bool(normalized) and any(marker in normalized for marker in allowed)
            and not any(marker in normalized for marker in denied)
        )
        if package_key in exceptions:
            permitted = permitted and exceptions[package_key].lower() in normalized
        entries.append({"name": name, "version": version, "license": licence, "permitted": permitted})
        if not permitted:
            failures.append(f"{name} {version}: {licence or 'no machine-readable licence'}")
        # Wheel metadata may contain a ``licenses/`` directory.  ``Path.glob``
        # also returns directories, while ``shutil.copyfile`` accepts only
        # regular files (notably aiosqlite uses that directory layout).
        files = [
            path
            for pattern in ("LICENSE*", "COPYING*")
            for path in info.glob(pattern)
            if path.is_file()
        ]
        license_dir = info / "licenses"
        if license_dir.exists():
            files.extend(path for path in license_dir.rglob("*") if path.is_file())
        for source in files:
            shutil.copyfile(source, args.notices_dir / f"{safe_name(name)}-{safe_name(version)}-{safe_name(source.name)}")

    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(entries, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if failures:
        print("Licence policy rejected package(s):\n  " + "\n  ".join(failures))
        return 1
    print(f"Licence policy accepted {len(entries)} installed package(s).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
