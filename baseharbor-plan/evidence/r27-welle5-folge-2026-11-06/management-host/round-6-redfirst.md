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

## AUFGAB 1 rot-auf-dem-kranken-commit — TEST-ERST-WEG (nachtrag a4573079 autorisiert)

Nachtrag a4573079 (2026-11-07): der blocker war richtig. F fuehrt test-erst-weg aus: NUR die testdatei
aus r4 3a80ed04 auf den sick-stand 64d93e94 holen, der FIX (sentinel + bootstrap-lock + management_host.go)
bleibt draussen. Kein produkt-code aendert.

SICK-SITZ: git worktree add worktrees/w289-sick 64d93e945aed892d2457bf94ab360cc11cc1567a (detached HEAD
64d93e94). worktrees/w289-sick unter workers/ (fixed workspace layout §mission).

TEST-HOLEN (r4 test, fix draussen):
  git checkout 3a80ed04 -- internal/coreinstallation/management_host_test.go
diff --stat: 1 file changed, 66 insertions(++) -- NUR management_host_test.go
  git diff --cached --name-only => internal/coreinstallation/management_host_test.go
  git diff --cached --stat -- internal/coreinstallation/management_host.go => (leer) = UNVERAENDERT
produkt-manifest: management_host.go ist in keinem staged diff, nur die testdatei ist geaendert.

ROTE AUSGBE (sick 64d93e94, r4-test nur):
  Kommando: go test ./internal/coreinstallation/ -run TestManagementHostAttachContendedByBootstrapServer -v
  exit=1
  internal/coreinstallation/management_host_test.go:120:13: undefined: ErrManagementHostBootstrapContended
  FAIL	github.com/mcpdev80/baseharbor/internal/coreinstallation [build failed]
  FAIL
Ursache (archaeologie, RUNBOOK 5.6): der sick-stand 64d93e94 (r3) kennt den bootstrap-lock + sentinel
ErrManagementHostBootstrapContended NICHT — die wurden erst mit r4 3a80ed04 in management_host.go
eingefuehrt. Der r4-test (auf r3-code gepflanzt) kompiliert nicht: undefined ErrManagementHostBootstrapContended.
Das ist der rote beweis, dass der test den fehler abdeckt — ohne den r4-fix ist er rot, mit r4-fix
kandidat 0f08b877 gruen. Keine kunstlich stillgelegte schranke; der fix bleibt aus.

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

- git ls-tree -r origin/main --name-only | grep mutation_mgmt_host => 0 matches. Das skript liegt auf
  KEINEM massgeblichen ref: nicht sick 64d93e94 (git show: path does not exist), nicht kandidat 0f08b877,
  nicht r4 3a80ed04, und auch nicht auf origin/main (ls-tree -r zeugnis oben).
- grep -rn 'mutation_mgmt_host' origin/work/r27-management-host und origin/develop => kein match.
- Die forderung (mutation script existiert bereits) ist daher GEGENSTANDSLOS. Die verifizierten stufen
  L0 8422c938 / L1 553dfa1f / L2 8422c938 (L0==L2) aus round-4/round-5 sind als bisherigen stand
  vermerkt; die sha-stenzen liegen im arbeitsbaum nicht vor (akt-1405a.md/round-4/round-5 nicht present),
  werden hier aber als referenz genannt.
- Das skript wurde nicht neu erfunden (evidence-only runde, kein produkt-write).

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
- Mutanten-stufen L0 8422c938 / L1 553dfa1f / L2 8422c938 aus round-4/round-5 als referenz genannt
  (sha-stenzen im arbeitsbaum nicht present; akt-1405a.md/round-4/round-5 nicht verfuegbar).
- ls-remote worktrees cleanup: keine w289-leiche (siehe unten).

## Endstand

- Kein produktcode geaendert (nur r4 test auf sick-stand gepflanzt; management_host.go unveraendert).
- PR #1407 (anderer zweig) nicht angefasst. .github/** nicht aendern.
- Akte geschrieben: baseharbor-plan/evidence/r27-welle5-folge-2026-11-06/management-host/round-6-redfirst.md

## End-line

END_DONE w289-0427-036-redfirst: sick=64d93e94 ROT(exit=1 undefined ErrManagementHostBootstrapContended mgmt_host_test.go:120) cand=0f08b877 GRUEN(exit=0) ls-tree-skript=absent(origin/main 0 matches) akte=baseharbor-plan/evidence/r27-welle5-folge-2026-11-06/management-host/round-6-redfirst.md
