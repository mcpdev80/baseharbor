"""Read-only prototype for resolving an already published CI evidence archive.

This is NOT a trusted run/provenance verifier and never grants qualification.
"""
import argparse
import hashlib
import json
import pathlib
import os
import stat
import re

HEX64 = re.compile(r"[0-9a-f]{64}\Z")
SAFE = re.compile(r"[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}\Z")
MAX_MANIFEST = 1024 * 1024
MAX_PAYLOAD = 8 * 1024 * 1024


class ArchiveError(ValueError):
    pass


def _read(path, maximum):
    """Read only regular files, refusing symlinks and oversized inputs."""
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError as exc:
        raise ArchiveError("missing or unsafe archive file") from exc
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > maximum:
            raise ArchiveError("unsafe or oversized archive file")
        with os.fdopen(fd, "rb", closefd=False) as stream:
            data = stream.read(maximum + 1)
        if len(data) > maximum:
            raise ArchiveError("archive file exceeds size bound")
        return data
    finally:
        os.close(fd)


def resolve(root, release, candidate_id, run_id, expected_digest):
    """Check archive byte integrity; no CI trust or current policy assessment."""
    if not all(SAFE.fullmatch(x) and x not in (".", "..") for x in (release, candidate_id, run_id)):
        raise ArchiveError("unsafe archive identity")
    if not HEX64.fullmatch(expected_digest):
        raise ArchiveError("invalid expected manifest digest")
    root = pathlib.Path(root)
    directory = root / "evidence" / release / candidate_id / run_id
    if any(part.is_symlink() for part in (root, root / "evidence", root / "evidence" / release,
                                         root / "evidence" / release / candidate_id, directory)):
        raise ArchiveError("unsafe archive directory")
    raw = _read(directory / "manifest.json", MAX_MANIFEST)
    if hashlib.sha256(raw).hexdigest() != expected_digest:
        raise ArchiveError("manifest digest mismatch")

    def reject_duplicate_keys(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise ArchiveError("duplicate manifest key")
            value[key] = item
        return value
    try:
        manifest = json.loads(raw, object_pairs_hook=reject_duplicate_keys)
    except (ValueError, UnicodeError) as exc:
        raise ArchiveError("invalid manifest JSON") from exc
    if not isinstance(manifest, dict) or manifest.get("schema") != "baseharbor.ci-evidence-archive/v1":
        raise ArchiveError("invalid manifest schema")
    candidate = manifest.get("candidate")
    run = manifest.get("run")
    if (manifest.get("release") != release or
            not isinstance(candidate, dict) or not isinstance(run, dict) or
            candidate.get("id") != candidate_id or run.get("archive_id") != run_id or
            not isinstance(run.get("id"), int) or isinstance(run.get("id"), bool) or
            run["id"] <= 0 or not isinstance(run.get("attempt"), int) or
            isinstance(run.get("attempt"), bool) or run["attempt"] <= 0 or
            not isinstance(manifest.get("checks"), list) or not manifest["checks"]):
        raise ArchiveError("manifest candidate/run identity or checks missing")
    if len(manifest["checks"]) > 128:
        raise ArchiveError("too many evidence checks")
    seen = set()
    check_ids = set()
    total_bytes = 0
    for check in manifest["checks"]:
        if (not isinstance(check, dict) or not isinstance(check.get("payload"), str) or
                not isinstance(check.get("id"), str) or not SAFE.fullmatch(check["id"])):
            raise ArchiveError("invalid evidence entry")
        if check["id"] in check_ids:
            raise ArchiveError("duplicate check identity")
        check_ids.add(check["id"])
        name = check["payload"]
        if (not name.startswith("payloads/") or name.count("/") != 1 or
                not SAFE.fullmatch(name.split("/")[1]) or name in seen):
            raise ArchiveError("unsafe or duplicate payload path")
        seen.add(name)
        digest = check.get("sha256")
        if not isinstance(digest, str) or not HEX64.fullmatch(digest):
            raise ArchiveError("invalid payload digest")
        if (directory / "payloads").is_symlink():
            raise ArchiveError("unsafe payload directory")
        content = _read(directory / name, MAX_PAYLOAD)
        total_bytes += len(content)
        if total_bytes > 32 * 1024 * 1024:
            raise ArchiveError("archive total payload size exceeded")
        if check.get("bytes") != len(content) or hashlib.sha256(content).hexdigest() != digest:
            raise ArchiveError("payload length or digest mismatch")
    return manifest


def main():
    parser = argparse.ArgumentParser(description="Read-only archive integrity check (not CI qualification)")
    parser.add_argument("root")
    parser.add_argument("release")
    parser.add_argument("candidate_id")
    parser.add_argument("run_id")
    parser.add_argument("expected_digest")
    args = parser.parse_args()
    try:
        manifest = resolve(args.root, args.release, args.candidate_id, args.run_id, args.expected_digest)
    except ArchiveError as exc:
        parser.exit(2, f"archive invalid: {exc}\n")
    print(json.dumps({"status": "integrity-only", "checks": len(manifest["checks"])}))


if __name__ == "__main__":
    main()
