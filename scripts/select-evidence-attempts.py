#!/usr/bin/env python3
"""Select the latest artifact attempt per logical gate; retain older evidence."""

import argparse
import pathlib
import re
import shutil


def select_attempts(root, run_id, families):
    if not re.fullmatch(r"[0-9]+", run_id):
        raise ValueError("source workflow run must be numeric")
    plans = []
    selected = []
    for family in families:
        if not re.fullmatch(r"[a-z][a-z0-9-]*", family):
            raise ValueError("invalid evidence family")
        source = root / family
        if source.is_symlink() or not source.is_dir():
            raise ValueError(f"missing canonical evidence family: {family}")
        groups = {}
        for directory in sorted(source.iterdir()):
            match = re.fullmatch(r"(.+)-([0-9]+)-([1-9][0-9]*)", directory.name)
            if directory.is_symlink() or not directory.is_dir() or not match:
                raise ValueError(f"unexpected evidence directory: {directory.name}")
            key, run, attempt = match.groups()
            if run != run_id:
                raise ValueError(f"wrong source run: {directory.name}")
            entries = groups.setdefault(key, {})
            number = int(attempt)
            if number in entries:
                raise ValueError(f"duplicate evidence attempt: {directory.name}")
            entries[number] = directory
        if not groups:
            raise ValueError(f"empty evidence family: {family}")
        for key, entries in sorted(groups.items()):
            latest = max(entries)
            selected.append((family, key, latest, entries[latest]))
            for attempt, directory in sorted(entries.items()):
                if attempt != latest:
                    archive = root / "superseded-evidence" / family / directory.name
                    if archive.exists() or archive.is_symlink():
                        raise ValueError(f"superseded evidence already exists: {directory.name}")
                    plans.append((directory, archive))
    # Validate the complete input before moving any evidence. Outcomes never
    # influence selection: a newer failure must supersede an older success.
    for directory, archive in plans:
        archive.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(directory), archive)
    return selected


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=pathlib.Path)
    parser.add_argument("run_id")
    parser.add_argument("families", nargs="+")
    args = parser.parse_args()
    try:
        selected = select_attempts(args.root, args.run_id, args.families)
    except (ValueError, OSError) as error:
        parser.exit(1, f"evidence selection failed: {error}\n")
    for family, key, attempt, _ in selected:
        print(f"{family}/{key}: selected attempt {attempt}")


if __name__ == "__main__":
    main()
