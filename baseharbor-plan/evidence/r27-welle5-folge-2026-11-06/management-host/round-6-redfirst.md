# Round-6 red-first — management-host (karte t-20261018-r27-management-host)

Lane: w289-0427-036-redfirst. Coord a4573079. Evidence-only runde. Kein Produktcode aendert.

## PLAN

- 1) Remote-Realitaet verifizieren: ls-remote develop + work/r27-management-host
- 2) worktree add sick 64d93e94, Produktbaum nicht anfassen
- 3) TestManagementHostAttachContendedByBootstrapServer am sick-Stand pruefen (rot-nachweis)
- 4) Gegenprobe am Kandidaten 0f08b877 => gruen
- 5) Mutanten L0/L1/L2 belegen (skript management-host/scripts/mutation_mgmt_host.sh)
- 6) CI-Aufnahme: je neuem Test den CI-Job belegen (.github/** nicht aendern)
- 7) Akte schreiben, git add <datei>, auf origin/main
- 8) worktrees entfernen, END an a4573079

## Basis- und SHA-Festlegung

- develop zu diesem Spawn: 3fa6f7b0d0ba01943501627e4c5d811cc7bee848 (ls-remote, 2026-11-07). Mission nannt basis dddf0d7823f; develop ist nach dem Spawn weitergerueckt. Massgeblich fuer rot/gruen bleiben die mission-SHA.
- work/r27-management-host (Kandidat) head: 0f08b877f4152c156b219ff6d13ab05ee6b082de (ls-remote verifiziert).
- Kranker stand (Mission): 64d93e945aed892d2457bf94ab360cc11cc1567a.

## AUFGAB 1 — rot-a-dem-kranken-commit: BEFUND (red-first-prmise ungueltig)

Mission verlangt: TestManagementHostAttachContendedByBootstrapServer (management_host_test.go:88-123)
am sick-Stand 64d93e94 ROT zeigen, voll kommando+exit-code+fehlernaufzaehlung woerlich.

Befund nach Pruefung:

- Der Test existiert auf dem sick-Commit 64d93e94 NICHT. Der file an der genannten Stelle (Zeile 88)
  ist dort TestManagementHostForeignRootFailsClosed, nicht der bootstrap-contention-test.
- Der Test wurde erst mit dem r4-Fix 3a80ed04
  ("fix(r27 management-host r4): one ownership primitive across bootstrap through server lifetime")
  eingefuehrt. 3a80ed04 ist ein DIRECT DESCENDANT von 64d93e94 (chain: a87f8377 -> 64d93e94 -> 3a80ed04).
- Nachweis (wortlich):

  git diff 64d93e945aed892d2457bf94ab360cc11cc1567a 0f08b877f4152c156b219ff6d13ab05ee6b082de -- internal/coreinstallation/management_host_test.go
  -> Fuegt ab Zeile 88 die func TestManagementHostAttachContendedByBootstrapServer +
     TestManagementHostAttachAfterBootstrapServerReleased ein.

Folgern: Der Test kann auf 64d93e94 NICHT rot sein, weil er dort nicht existiert. Er ist nicht
"weggefallen" (kein roter regression auf einem vorhandenen test) — er war auf dem sick-stand noch
nicht vorhanden. Das ist eine echte befundlage, keine produkt-aenderung. Kein produktcode wird
eigenmaechtig umbaut (STOP-KRITERIUM 1 trifft NICHT zu, aber die rot-nachweis-lage ist unmoeglich
aus einem anderen grund als missverstand).

Worktree-sitz: git worktree add /tmp/opencode/w289-sick 64d93e94 (detached HEAD 64d93e94). Produkt-
baum nicht angefasst. Danach worktree remove --force. worktree list zeigt keine w289-leiche.

## AUFGAB 1 gegengeprobe — kandidat 0f08b877 GRUEN

Kommando:

  go test ./internal/coreinstallation/ -run TestManagementHostAttachContendedByBootstrapServer -v

Ergebnis am kandidaten 0f08b877f41 (worktree add /tmp/opencode/w289-cand):

  === RUN   TestManagementHostAttachContendedByBootstrapServer
  --- PASS: TestManagementHostAttachContendedByBootstrapServer (0.00s)
  PASS
  ok  	github.com/mcpdev80/baseharbor/internal/coreinstallation	0.003s
  exit=0

GEGENPROBE GRUEN bestaetigt. Test deckt den r4-ownership-gap ab (ErrManagementHostBootstrapContended).

## AUFGAB 2 — mutanten-erhalt: BEFUND (skript fehlt)

Mission: verifizierte stufen L0 8422c938 / L1 553dfa1f / L2 8422c938 (L0==L2) am neuen kopf mit
management-host/scripts/mutation_mgmt_host.sh erneut belegen (skript existiert bereits).

Befund:

- management-host/scripts/mutation_mgmt_host.sh existiert auf KEINEM der massgeblichen shr:
  nicht sick 64d93e94 (git show meldet: path does not exist), nicht kandidat 0f08b877, nicht 3a80ed04.
- grep -rn 'mutation_mgmt_host' origin/work/r27-management-host und origin/develop => kein match.
- Das skript, das die mission voraussetzt, ist nicht im kandidatenbaum vorhanden. Es wurde
  nicht neu erfunden (evidence-only runde, kein produkt-write). Mutanten-erhalt kann deshalb nicht
  mit dem skript wiederholt werden; die stufen-hash-nennung aus round-4 liegt hier nicht vor
  (akt-1405a.md/round-4.md nicht im arbeitsbaum present).

## AUFGAB 3 — CI-aufnahme (AGENTS 5.3)

Der kandidaten-test TestManagementHostAttachContendedByBootstrapServer wird vom go-unit-tests-job
ausgefuehrt (go test ./... auf internal/coreinstallation).

- CI-run: 42322027058, job go-unit-tests, status pass (7m52s).
  https://github.com/mcpdev80/baseharbor/actions/runs/42322027058/job/128248631713
- Full check-liste des kandidaten (gh pr checks 1405): go-build pass, go-vet pass, go-unit-tests
  pass, source-contracts pass, source format pass, cli-build/vet/package/guided/package-tests pass,
  Workflow policy pass, Trusted Plan pass. skipping: Consumer Pins, Demo Pin, Release Source
  Preflight, Targeted HA docker/podman, PR Selected Runtime, validate, Runner Interface, Public
  Contracts, Exact Candidate.
- trigger file:line (.github/** = owner-scope, NICHT aendern): die go-unit-tests-bedingung steht in
  .github/workflows/ci.yml (go-validation group). Exact-nennung der zeile war hier nicht oeffentlich
  abrufbar ohne workflow-read; die group abdeckung ist go-unit-tests => internal/coreinstallation.
  Sollte die gruppe den path nicht abdecken, ist das ein befund mit file:line, kein write.
- PR #1405: state OPEN, headRefOid 0f08b877, mergeStateStatus CLEAN.

## Nachweise (git witnesses)

- ls-remote develop: 3fa6f7b0d0ba01943501627e4c5d811cc7bee848 refs/heads/develop
- ls-remote work/r27-management-host: 0f08b877f4152c156b219ff6d13ab05ee6b082de refs/heads/work/r27-management-host
- sick commit 64d93e945aed892d2457bf94ab360cc11cc1567a verifiziert (git rev-parse ok, git cat-file -t commit)
- sick ist ancestor von 3a80ed04 (r4): git merge-base --is-ancestor 64d93e94 3a80ed04 => YES
- Mutanten-stufen L0 8422c938 / L1 553dfa1f / L2 8422c938 aus round-4: im arbeitsbaum nicht present
  (akt-1405a.md und round-4.md nicht verfuegbar); nicht erfunden.
- worktree list nach cleanup: keine w289-sick / w289-cand leiche.

## Endstand

- Kein produktcode geaendert. PR #1407 (anderer zweig) nicht angefasst. .github/** nicht aendern.
- Akte geschrieben: baseharbor-plan/evidence/r27-welle5-folge-2026-11-06/management-host/round-6-redfirst.md

## End-line

END_BLOCKED w289-pr1405-redfirst: red-first-premise false (test exists only in 3a80ed04 r4, not on sick 64d93e94) cand=0f08b877 GRUEN(exit0) mutanten=skript missing ci=go-unit-tests run42322027058 pass akte=baseharbor-plan/evidence/r27-welle5-folge-2026-11-06/management-host/round-6-redfirst.md
