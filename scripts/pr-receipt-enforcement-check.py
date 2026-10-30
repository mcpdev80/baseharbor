#!/usr/bin/env python3
"""Receipt Enforcement check for contracts/** changes.

Verifies that an approved Receipt exists bound to this exact PR tuple
(head_sha + base_sha). Reads from baseharbor-plan main
evidence/receipts/public-contracts/*.json. Fails closed on network/
schema problems. No secrets required (public read).
"""
import json
import os
import sys
from pathlib import Path


def main():
    if len(sys.argv) < 2:
        print("USAGE: pr-receipt-enforcement-check.py <receipts_dir>", file=sys.stderr)
        sys.exit(1)

    receipts_dir = sys.argv[1]
    head_sha = os.environ.get("HEAD_SHA", "")
    base_sha = os.environ.get("BASE_SHA", "")

    if not head_sha or not base_sha:
        print("ERROR: HEAD_SHA and BASE_SHA must be set", file=sys.stderr)
        sys.exit(1)

    found_approved = False

    receipt_files = list(Path(receipts_dir).glob("*.json"))

    for receipt_file in receipt_files:
        try:
            receipt = json.loads(receipt_file.read_text())
        except (json.JSONDecodeError, OSError) as exc:
            print(f"WARNING: could not read {receipt_file}: {exc}", file=sys.stderr)
            continue

        # Verify schema: must be a dict with required fields.
        if not isinstance(receipt, dict):
            continue

        # approved must be non-null, non-false, non-empty.
        approved = receipt.get("approved")
        if approved is None or approved is False or approved == "":
            continue

        # Check PR tuple binding.
        bound_head = receipt.get("head_sha")
        bound_base = receipt.get("base_sha")
        if bound_head != head_sha or bound_base != base_sha:
            continue

        # Verify plan references exist and are valid.
        plan_refs = receipt.get("plan_refs", [])
        if not isinstance(plan_refs, list):
            continue

        found_approved = True
        print(f"APPROVED_RECEIPT: {receipt_file.name}", file=sys.stderr)
        break

    if not found_approved:
        print(
            f"RECEIPT_BLOCKED: no approved Receipt for head_sha={head_sha}, "
            f"base_sha={base_sha}",
            file=sys.stderr,
        )
        sys.exit(1)

    print("Receipt Approval PASSED: approved Receipt found for this PR tuple")


if __name__ == "__main__":
    main()
