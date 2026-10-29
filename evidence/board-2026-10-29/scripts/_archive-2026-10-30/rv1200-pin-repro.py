#!/usr/bin/env python3
"""Read-only reproduction against exact committed PR objects; no network/push."""
import os
import subprocess
import tempfile
from pathlib import Path

root = Path(__file__).resolve().parents[3] / 'baseharbor'
head = '5aaa38ece21c40ad10d2805bf0bc41fac3d695b8'
pin = 'b53632aaaee6067397c27000c229f34736ec2155'
raw = subprocess.check_output(['git', '-C', str(root), 'show', head + ':scripts/receipt_verifier.py'])
with tempfile.TemporaryDirectory(prefix='rv1200-') as temp:
    module = Path(temp) / 'receipt_verifier.py'
    module.write_bytes(raw)
    scope = {'__file__': str(module), '__name__': 'rv1200_review'}
    exec(compile(raw, str(module), 'exec'), scope)
    for source, should_pass in [(pin, True), (head, False)]:
        os.environ['GITHUB_SHA'] = source
        try:
            scope['pinned_source'](root, pin)
        except scope['ReceiptError'] as exc:
            print(source, 'BLOCKED:', exc)
            assert not should_pass
            assert str(exc) == 'running approval workflow differs from reviewed pinned definition'
        else:
            print(source, 'PASS')
            assert should_pass
