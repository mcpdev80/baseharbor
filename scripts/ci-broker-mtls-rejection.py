#!/usr/bin/env python3
"""Fail unless curl reached and verified the target and received a TLS client-cert alert."""
import ipaddress
import re
import sys
from pathlib import Path


CLIENT_CERT_ALERT = re.compile(
    r"\b(?:sslv3|tlsv1(?:\.\d)?|tlsv13) alert "
    r"(?:bad certificate|unknown ca|certificate required|access denied)\b",
    re.IGNORECASE,
)


def rejected(expected_ip: str, exit_code: int, metadata: str, stderr: str) -> bool:
    try:
        remote_ip, verify_result, http_code = metadata.strip().split("|")
        same_ip = ipaddress.ip_address(remote_ip) == ipaddress.ip_address(expected_ip)
    except ValueError:
        return False
    return (
        same_ip
        and exit_code in (35, 56)
        and verify_result == "0"
        and http_code == "000"
        and CLIENT_CERT_ALERT.search(stderr) is not None
    )


def main() -> int:
    try:
        target, code, metadata, error = sys.argv[1:]
        success = rejected(target, int(code), Path(metadata).read_text(), Path(error).read_text())
    except (ValueError, OSError):
        success = False
    if not success:
        print("cross-app request did not prove a verified target and client certificate rejection", file=sys.stderr)
        return 1
    print("verified target rejected the other application's client certificate")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
