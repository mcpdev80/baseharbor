"""Isolated create-only evidence staging prototype; NOT a trusted CI publisher."""
import hashlib
import json
import os
from pathlib import Path
import stat
from r27_archive_resolver import ArchiveError, SAFE, resolve

MAX_PAYLOAD = 8 * 1024 * 1024
MAX_TOTAL = 32 * 1024 * 1024


def publish(root, release, candidate_id, run_id, manifest, payloads):
    """Stage checked blobs before manifest; caller owns authentication/redaction."""
    if not all(isinstance(v, str) and SAFE.fullmatch(v) and v not in (".", "..")
               for v in (release, candidate_id, run_id)):
        raise ArchiveError("unsafe archive identity")
    if not isinstance(manifest, dict) or not isinstance(payloads, dict) or not payloads:
        raise ArchiveError("invalid archive input")
    if (manifest.get("release") != release or
            not isinstance(manifest.get("candidate"), dict) or
            manifest["candidate"].get("id") != candidate_id or
            not isinstance(manifest.get("run"), dict) or
            manifest["run"].get("archive_id") != run_id):
        raise ArchiveError("manifest identity mismatch")
    checks = manifest.get("checks")
    if not isinstance(checks, list) or not checks or len(checks) != len(payloads) or len(checks) > 128:
        raise ArchiveError("invalid payload inventory")
    names = set()
    check_ids = set()
    total = 0
    for check in checks:
        if not isinstance(check, dict):
            raise ArchiveError("invalid check")
        identity = check.get("id")
        if not isinstance(identity, str) or not SAFE.fullmatch(identity) or identity in check_ids:
            raise ArchiveError("missing or duplicate check identity")
        check_ids.add(identity)
        name = check.get("payload")
        if (not isinstance(name, str) or not name.startswith("payloads/") or
                name.count("/") != 1 or not SAFE.fullmatch(name.split("/")[1]) or name in names):
            raise ArchiveError("unsafe or duplicate payload")
        names.add(name)
        data = payloads.get(name)
        if not isinstance(data, bytes) or len(data) > MAX_PAYLOAD:
            raise ArchiveError("missing or oversized payload")
        total += len(data)
        if total > MAX_TOTAL or check.get("bytes") != len(data) or check.get("sha256") != hashlib.sha256(data).hexdigest():
            raise ArchiveError("payload size or digest mismatch")
    if set(payloads) != names:
        raise ArchiveError("unlisted payload")
    raw = json.dumps(manifest, sort_keys=True, separators=(",", ":"), allow_nan=False).encode()
    if len(raw) > 1024 * 1024:
        raise ArchiveError("oversized manifest")
    digest = hashlib.sha256(raw).hexdigest()
    root = Path(root)
    directory = root / "evidence" / release / candidate_id / run_id
    for parent in (root, root / "evidence", root / "evidence" / release,
                   root / "evidence" / release / candidate_id, directory, directory / "payloads"):
        if parent.is_symlink():
            raise ArchiveError("unsafe archive directory")
        parent.mkdir(parents=True, exist_ok=True)
        if not parent.is_dir():
            raise ArchiveError("non-directory archive component")

    def create_once(path, data):
        try:
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        except FileExistsError:
            if path.is_symlink() or not path.is_file():
                raise ArchiveError("archive identity collision")
            from r27_archive_resolver import _read
            if _read(path, max(len(data), 1)) != data:
                raise ArchiveError("archive identity collision")
            return
        try:
            with os.fdopen(fd, "wb") as stream:
                stream.write(data)
                stream.flush()
                os.fsync(stream.fileno())
        except BaseException:
            path.unlink(missing_ok=True)
            raise

    for name, data in payloads.items():
        create_once(directory / name, data)
    create_once(directory / "manifest.json", raw)
    resolve(root, release, candidate_id, run_id, digest)
    return digest
