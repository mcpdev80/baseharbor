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


def _read(path, maximum, *, parent_fd=None):
    """Read a bounded regular file relative to a verified directory descriptor."""
    try:
        flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
        fd = os.open(path, flags, dir_fd=parent_fd)
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


def _open_directory_chain(root, parts):
    """Pin every ancestor using O_NOFOLLOW, resisting symlink replacement."""
    fds = []
    try:
        anchor = os.path.abspath(os.fspath(root))
        components = pathlib.Path(anchor).parts
        fd = os.open(components[0], os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        fds.append(fd)
        for component in (*components[1:], *parts):
            next_fd = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                              dir_fd=fd)
            fds.append(next_fd)
            fd = next_fd
        return fds
    except OSError as exc:
        for opened in reversed(fds):
            os.close(opened)
        raise ArchiveError("unsafe archive directory") from exc


def resolve(root, release, candidate_id, run_id, expected_digest):
    """Check archive byte integrity; no CI trust or current policy assessment."""
    if not all(isinstance(x, str) and SAFE.fullmatch(x) and x not in (".", "..")
               for x in (release, candidate_id, run_id)):
        raise ArchiveError("unsafe archive identity")
    if not isinstance(expected_digest, str) or not HEX64.fullmatch(expected_digest):
        raise ArchiveError("invalid expected manifest digest")
    root = pathlib.Path(root)
    directory = root / "evidence" / release / candidate_id / run_id
    fds = _open_directory_chain(root, ("evidence", release, candidate_id, run_id))
    try:
        raw = _read("manifest.json", MAX_MANIFEST, parent_fd=fds[-1])
    finally:
        for fd in reversed(fds):
            os.close(fd)
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
        manifest = json.loads(
            raw, object_pairs_hook=reject_duplicate_keys,
            parse_constant=lambda value: (_ for _ in ()).throw(
                ArchiveError("nonfinite manifest number")))
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
        payload_fds = _open_directory_chain(root, ("evidence", release, candidate_id,
                                                   run_id, "payloads"))
        try:
            content = _read(name.split("/")[1], MAX_PAYLOAD, parent_fd=payload_fds[-1])
        finally:
            for fd in reversed(payload_fds):
                os.close(fd)
        total_bytes += len(content)
        if total_bytes > 32 * 1024 * 1024:
            raise ArchiveError("archive total payload size exceeded")
        if (type(check.get("bytes")) is not int or check["bytes"] != len(content) or
                hashlib.sha256(content).hexdigest() != digest):
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
