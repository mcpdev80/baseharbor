## PLAN

1. Read receipt-approval.yml at 96011aac8 (116 lines)
2. Diagnose: shadow job references `needs.approve.outputs.receipt`/`plan`/`candidate` but approve job has no `outputs:` block → actionlint reports `inputs {}` empty
3. Fix: add `receipt`, `plan`, `candidate` to `workflow_dispatch.inputs` block with `required:false`, `type:string`, descriptions matching receipt-shadow.yml
4. Fix: change shadow job to reference `inputs.receipt`/`inputs.plan`/`inputs.candidate` instead of `needs.approve.outputs.*`
5. Verify: actionlint 0 findings on .github/workflows/receipt-approval.yml and .github/workflows/receipt-shadow.yml
6. Verify: python3 -m unittest discover -s scripts -p 'test_receipt*.py' → 120 tests (2 pre-existing failures unrelated to fix)
7. Evidence: ordinary commit on work/r27-fix-approval-inputs branch

## actionlint Kommando + Ausgabe

```bash
cd baseharbor && GOBIN="/tmp/ai-bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
/tmp/ai-bin/actionlint -shellcheck= .github/workflows/receipt-approval.yml
```

Ergebnis: `EXIT: 0` — 0 findings.

```bash
/tmp/ai-bin/actionlint -shellcheck= .github/workflows/receipt-shadow.yml
```

Ergebnis: `EXIT: 0` — 0 findings.

## Test-Zaehlung

```bash
cd baseharbor && python3 -m unittest discover -s scripts -p 'test_receipt*.py'
```

Ergebnis: `Ran 120 tests in 51.448s` — 118 passed, 2 pre-existing failures (BF-W3-1 fixture errors, unrelated to this fix).

## END X1-DONE

SHA: 1b1721b96a7e375fbb5b8305aae308d0c4ac2ae9
PR: none (ordinary commit on work/r27-fix-approval-inputs, pushed to origin)
ls-remote-Beweis:
  $ git ls-remote origin refs/heads/work/r27-fix-approval-inputs
  1b1721b96a7e375fbb5b8305aae308d0c4ac2ae9 refs/heads/work/r27-fix-approval-inputs
  d8cb47661f38451519295d0f347958abd2b7af44
Owner-Regel: KEINE GITHUB_* Overrides eingefuehrt; inputs-Block explizit deklariert.
