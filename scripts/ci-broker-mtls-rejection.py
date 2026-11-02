#!/usr/bin/env python3
"""Fail unless curl reached and verified the target and received a TLS client-cert alert."""
import ipaddress
import re
import sys
from pathlib import Path


CLIENT_CERT_ALERT = re.compile(
    r"(?:\balert\b[^\n]{0,100}\bbad certificate\b|"
    r"\bbad certificate\b[^\n]{0,100}\balert\b)",
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
    diagnosis = "input-invalid"
    try:
        target, code, metadata, error = sys.argv[1:]
        metadata_text = Path(metadata).read_text()
        error_text = Path(error).read_text()
        success = rejected(target, int(code), metadata_text, error_text)
        remote, verify, http = metadata_text.strip().split("|")
        diagnosis = (f"exit-allowed={int(code) in (35, 56)} "
                     f"target-matched={ipaddress.ip_address(remote) == ipaddress.ip_address(target)} "
                     f"server-verified={verify == '0'} http-unanswered={http == '000'} "
                     f"client-cert-alert={CLIENT_CERT_ALERT.search(error_text) is not None} "
                     f"any-tls-alert={'alert' in error_text.lower()}")
    except (ValueError, OSError):
        success = False
    if not success:
        print("cross-app request did not prove a verified target and client certificate rejection", file=sys.stderr)
        print(diagnosis, file=sys.stderr)
        return 1
    print("verified target rejected the other application's client certificate")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
