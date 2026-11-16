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


def identity_rejected_by_server(server_log: str, client_ip: str) -> bool:
    try:
        expected = ipaddress.ip_address(client_ip)
        for line in server_log.splitlines():
            match = re.search(r"TLS handshake error from (\[[^]]+\]|[^:\s]+):[0-9]+: (.*)", line)
            if match and ipaddress.ip_address(match.group(1).strip("[]")) == expected and (
                "runtime client certificate identity does not match this application broker" in match.group(2)
            ):
                return True
    except ValueError:
        return False
    return False


def rejected(expected_ip: str, client_ip: str, exit_code: int, metadata: str, stderr: str, server_log: str) -> bool:
    try:
        remote_ip, verify_result, http_code = metadata.strip().split("|")
        same_ip = ipaddress.ip_address(remote_ip) == ipaddress.ip_address(expected_ip)
    except ValueError:
        return False
    return (
        same_ip
        and 0 < exit_code <= 255
        and verify_result == "0"
        and http_code == "000"
        and CLIENT_CERT_ALERT.search(stderr) is not None
        and identity_rejected_by_server(server_log, client_ip)
    )


def main() -> int:
    diagnosis = "input-invalid"
    try:
        target, client, code, metadata, error, server_log = sys.argv[1:]
        metadata_text = Path(metadata).read_text()
        error_text = Path(error).read_text()
        success = rejected(target, client, int(code), metadata_text, error_text, Path(server_log).read_text())
        remote, verify, http = metadata_text.strip().split("|")
        diagnosis = (f"exit-code={int(code)} exit-allowed={0 < int(code) <= 255} "
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
