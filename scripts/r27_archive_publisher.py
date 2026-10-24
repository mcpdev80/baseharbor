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
    run = manifest["run"]
    if (manifest.get("schema") != "baseharbor.ci-evidence-archive/v1" or
            type(run.get("id")) is not int or run["id"] <= 0 or
            type(run.get("attempt")) is not int or run["attempt"] <= 0):
        raise ArchiveError("invalid archive schema or run provenance fields")
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
        if (total > MAX_TOTAL or type(check.get("bytes")) is not int or
                check["bytes"] != len(data) or
                check.get("sha256") != hashlib.sha256(data).hexdigest()):
            raise ArchiveError("payload size or digest mismatch")
    if set(payloads) != names:
        raise ArchiveError("unlisted payload")
    raw = json.dumps(manifest, sort_keys=True, separators=(",", ":"), allow_nan=False).encode()
    if len(raw) > 1024 * 1024:
        raise ArchiveError("oversized manifest")
    digest = hashlib.sha256(raw).hexdigest()
    root = Path(root)
    directory = root / "evidence" / release / candidate_id / run_id
    # Create and pin each directory via its parent descriptor; never follow
    # a substituted symlink during publication.
    from r27_archive_resolver import _read
    chain = []
    try:
        anchor = os.path.abspath(os.fspath(root))
        parts = Path(anchor).parts
        fd = os.open(parts[0], os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        chain.append(fd)
        for part in (*parts[1:], "evidence", release, candidate_id, run_id, "payloads"):
            try:
                os.mkdir(part, mode=0o700, dir_fd=fd)
            except FileExistsError:
                pass
            fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                         dir_fd=fd)
            chain.append(fd)
        directory_fd, payload_fd = chain[-2], chain[-1]

        def create_once(name, data, parent_fd):
            try:
                file_fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL |
                                  os.O_NOFOLLOW, 0o600, dir_fd=parent_fd)
            except FileExistsError:
                if _read(name, max(len(data), 1), parent_fd=parent_fd) != data:
                    raise ArchiveError("archive identity collision")
                return
            try:
                with os.fdopen(file_fd, "wb") as stream:
                    stream.write(data)
                    stream.flush()
                    os.fsync(stream.fileno())
            except BaseException:
                os.unlink(name, dir_fd=parent_fd)
                raise

        try:
            existing = _read("manifest.json", 1024 * 1024,
                             parent_fd=directory_fd)
        except ArchiveError as exc:
            # Only absence is an allowed first publication; unsafe existing
            # files must not be treated as missing.
            try:
                os.stat("manifest.json", dir_fd=directory_fd,
                        follow_symlinks=False)
            except FileNotFoundError:
                existing = None
            else:
                raise ArchiveError("archive identity collision") from exc
        if existing is not None and existing != raw:
            raise ArchiveError("archive identity collision")
        for name, data in payloads.items():
            create_once(name.split("/")[1], data, payload_fd)
        create_once("manifest.json", raw, directory_fd)
    except OSError as exc:
        raise ArchiveError("unsafe archive publication directory") from exc
    finally:
        for opened in reversed(chain):
            os.close(opened)
    resolve(root, release, candidate_id, run_id, digest)
    return digest
