#!/usr/bin/env python3
"""Install pre-resolved wheels by extraction without running target Python."""
from __future__ import annotations

import argparse
import zipfile
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--wheels", type=Path, required=True)
    parser.add_argument("--target", type=Path, required=True)
    args = parser.parse_args()
    args.target.mkdir(parents=True, exist_ok=True)
    wheels = sorted(args.wheels.glob("*.whl"))
    if not wheels:
        raise SystemExit("no wheels to install")
    for wheel in wheels:
        with zipfile.ZipFile(wheel) as archive:
            archive.extractall(args.target)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
