# D3 mergecheck — v3.6 Kette (READ-ONLY merge-tree Prognose)

Lane: d3 · Auftrag: evidence/board-2026-10-22/prompts/d3.txt · Datum: 2026-10-27
Modus: rein lesend (`git merge-tree --write-tree`, `cat-file`, `diff`). Kein Checkout,
kein Commit, kein Push. Einzige geschriebene Datei = dieses Root-Dossier.

## Root

Kette für v3.6 (alle Feature-Baume basieren direkt auf v35; merge-base = f57b30ed, behind=0):

| Slot | Branch (work/r27-*) | SHA (IMMER ls-remote frisch) | Commit-Subjekt |
|------|---------------------|------------------------------|----------------|
| v35 (Base) | work/r27-correction-v35 | `f57b30ed` | Merge commit '1d8237e9' into HEAD |
| f1b = **w4a** | work/r27-fix-f1b-recovery | `75b30b32` | w4a: F3 plan-ancestry, F5 schema-valid nested evidence + sha40 |
| akte2b = **w4b3** | work/r27-fix-f2-akte2b | `3dcc58c7` | docs(evidence): w4b3 BLOCK F2 forward commit + PR #1131 |
| outputs2 = **w4c** | work/r27-fix-f6-outputs2 | `418acbfd` | docs(evidence): w4c PR 1132 delivery reference |

Frische-Warnung (vom Auftrag explizit verlangt, "w4a evtl. neuer — IMMER ls-remote frisch"):
Der Auftrag nannte f1b = `88a2f15a`. Der frische `ls-remote` liefert **`75b30b32`** — w4a ist
vorgerückt. Alle Berechnungen oben verwenden den FRISCHEN SHA. v35/akte2b/outputs2 stimmen mit
den Auftragsangaben überein.

## PLAN

1. [x] BOOT: pwd/ls + `git fetch origin` der 4 work/r27-* Branches — ok (kein D3-BOOTFAIL).
2. [x] `ls-remote` frisch für alle 4 Slots — f1b-Delta 88a2f15a→75b30b32 erkannt.
3. [x] `merge-tree --write-tree` je Paar (6 Paare).
4. [x] Kumulativ in Reihenfolge f1b → akte2b → outputs2 (via `--merge-base=v35` + Result-Tree-Kette).
5. [x] Konflikt: Datei + beide Hunk-Seiten + patch-id-Überlappung.
6. [x] Signaturvergleich Writer-Funktionen in w4a/w4b3/w4c → Staging-Risiko-Liste.
7. [x] Write-Set-Mengen (Disjunktheit) je Paar gegen v35.
8. [x] Dossier +Matrix +Empfehlung. Kein checkout/commit/push außer diesem Dossier.

## Matrix — paarweise + kumulativ

| Merge | Ergebnis | Tree |
|-------|----------|------|
| v35 × f1b | CLEAN (ff-artig, f1b ⊇ v35) | 7998ecc0 |
| v35 × akte2b | CLEAN | 7cd2fe27 |
| v35 × outputs2 | CLEAN | 30fb56bd |
| f1b × akte2b | CLEAN | 624e50db |
| **f1b × outputs2** | **CONFLICT** scripts/test_receipt_verifier.py | 4909689a |
| akte2b × outputs2 | CLEAN | ad686b1e |
| **KUMULATIV v35+f1b+akte2b+outputs2** | **CONFLICT** scripts/test_receipt_verifier.py | 60ca0e0f |

Konfliktquelle ist ausschließlich das Paar **w4a × w4c**; akte2b (w4b3) liefert nichts dazu
(kumulativer Konflikt == paarweiser f1b×outputs2, identische Stage-Blobs).

Write-Set-Überlappungen (gegen v35):

| Paar | gemeinsame Dateien |
|------|--------------------|
| w4a × w4b3 | KEINE (disjunkt) |
| w4b3 × w4c | KEINE (disjunkt) |
| w4a × w4c | `scripts/receipt_verifier.py`, `scripts/test_receipt_verifier.py` |

Workflow-Dateien disjunkt: w4a = receipt-post-run.yml; w4c = receipt-approval.yml + receipt-shadow.yml.

## Konflikt im Detail: scripts/test_receipt_verifier.py

Stage-1 (base/v35) `664f8f9c` · Stage-2 (ours = w4a) `2204b6fc` · Stage-3 (theirs = w4c) `6aa77ae6`
patch-id ours = `3edb95da9e3a` · patch-id theirs = `9bf0adfaabdd` → **unterschiedliche Patches,
die dieselben Regionen berühren** → echter Content-Konflikt (kein Whitespace-Artefakt).

Vier Basis-Hunk-Cluster, in denen beide Seiten dieselbe Region umschreiben:

1. Import-Block (~Zl. 26): w4a +`import receipt_tasklink as tl` (Zl. 27) vs. w4c +`import yaml`
   (Zl. 26) — benachbart, gleicher Anker.
2. Fixture setUp (~Zl. 69 vs. 108/116): BEIDE richten ein Bare-`origin`-Remote ein —
   w4a: `self.bare` + `git init --bare` + `checkout -b main` + `push HEAD:refs/heads/main` inline;
   w4c: `self.planning_remote` + `remote add origin` + neue Helfer `seed_remote_main()` / `remote_main()`.
   Zwei unvereinbare "origin"-Konventionen in derselben Fixture-Klasse.
3. PublishInterfaceTest (~Zl. 702–718): w4a benennt `test_missing_publish_interface_blocks_publication`
   → `test_publish_interface_exists_and_is_used` (BF-W3-1 als gelöst); w4c BEHÄLT die Block-Test-
   Erwartung und ergänzt Credential-/Push-Scope-Tests. Widersprüchliche Erwartung an derselben Stelle.
4. ProductionPathCliTest (~Zl. 938–951): w4a schreibt `..._fails_closed_on_missing_publish` →
   `..._publishes_card_durably` um; w4c ergänzt um dieselbe erhaltene fail-closed-Test herum
   `test_approve_cli_subprocess_emits_exact_resolved_tuple`. Dieselbe Region, zwei Umbauten.

## Staging-Risiko-Liste — Writer-Funktionen (w4a/w4b3/w4c Signaturvergleich)

Ergebnis: **Die Writer-Funktionssignaturen divergieren NICHT zwischen den drei Bäumen.**

| Funktion | v35 | w4a | w4b3 | w4c | Divergenz |
|----------|-----|-----|------|-----|-----------|
| `receipt_writer.publish(root,path,local_commit,*,dry_run=False)->Tuple[str,str,str]` | ja | identisch | identisch | identisch | keine |
| `receipt_tasklink.publish_evidence(...,render_fn=None,check_fn=None,_git_fn=None)` | ja | identisch | identisch | identisch | keine |
| `_git_fn`-Parameter in publish_evidence | vorhanden (9×) | vorhanden | vorhanden | vorhanden | keine |
| `receipt_writer.write(...)` | `(root,plan,run,attempt,api=None)` | +`*, product=None` | wie v35 | wie v35 | **additiv, w4a-only** |
| w4a-intern neu: `_is_real_remote`, `_push_authed`, `_scrub_secret` | — | neu | — | — | w4a-only, keine Kollision |

Interpretation:
- `product` an `write()` ist keyword-only mit Default `None` → abwärtskompatibel; Aufrufer ohne
  `product` brechen nicht. Im merged Baum ist `receipt_writer.py` = w4a-Version (nur w4a ändert sie).
- w4c ruft `publish_evidence(..., _git_fn=writer_push_git(...))` — `_git_fn` ist in **allen** Bäumen
   vorhanden → im gemergten Baum (tasklink = w4a, unverändert an der Signatur) aufrufkompatibel.
- **Kein Writer-Signatur-Stagingrisiko.** Das eigentliche Stagingrisiko liegt NICHT in den
  Writer-Funktionen, sondern in (a) dem harten Test-Fixture-Konflikt und (b) dem nächsten Punkt:

## Verdecktes Risiko: scripts/receipt_verifier.py (textuell CLEAN, semantisch doppelt umgebaut)

`merge-tree` meldet `receipt_verifier.py` in f1b×outputs2 UND kumulativ als **Auto-merge CLEAN**
(keine Conflict-Marker), weil sich die Zeilenbereiche der beiden Umbauten textuell nicht überschneiden.
Der automerged Baum enthält aber GLEICHZEITIG:
- w4a-Neuerungen: `extract_facts()`, überarbeitete `approve()`/`publish_approval()`-Rückgabefelder;
- w4c-Neuerungen: `front_matter()`, `task_pr_associations()`, `evidence_reference()`,
  `writer_push_git()`, sowie eine andere `task_binding()`-/`approve_pr()`-/`publish_approval()`-Fassung.

Die gemeinsamen Funktionen `task_binding` (w4a Zl. 473 / w4c Zl. 467) und `publish_approval`
(w4a Zl. 513 / w4c Zl. 535) wurden von beiden Zweigen UNTERSCHIEDLICH umgebaut und vom Merge
zeilenweise verschmolzen. Ein grüner Auto-Merge-Gate beweist hier NICHT die Lauffähigkeit —
diese Kombination wurde nie als Einheit getestet. Zwingend: gemergten Verifier + gemergte
Testsuite auf dem echten SHA laufen lassen, nicht auf "clean auto-merge" vertrauen.

## Empfehlung

1. **v3.6 ist keine sauber sequentielle/grüne Kette.** w4a × w4c erzeugen einen harten
   Content-Konflikt in `scripts/test_receipt_verifier.py`; ein Auto-Merge der vollen Kette scheitert.
2. **w4b3 (akte2b) ist orthogonal** (disjunkte Write-Sets, merge-clark mit allem, kein Konflikt) →
   kann zuerst/gefahrlos mitgenommen werden.
3. **Vor v3.6-Reif** muss `scripts/test_receipt_verifier.py` zwischen w4a und w4c händisch
   reconciliert werden:
   - EINE origin-Remote-Konvention wählen (w4a inline `self.bare`+checkout/push vs. w4c
     `self.planning_remote`+`seed_remote_main()`/`remote_main()`); die andere Verankerung entfernen.
   - `PublishInterfaceTest` + `ProductionPathCliTest` zusammenführen: geklärtes BF-W3-1-Verhalten
     (publish existiert / "fails_closed" vs. "publishes_card_durably") plus w4c Credential-/Push-
     Scope-Tests, ohne doppelte sich widersprechende Erwartung.
4. **`receipt_verifier.py` trotz cleanem Auto-Merge als Einheit neu testen** (Punkt oben); nicht auf
   dem Merge-Text verlassen.
5. Integration in dieser Reihenfolge vorschlagen: v35 + akte2b (frei) → dann w4a + w4c mit
   explizitem Reconcile-Schritt für die Test-Fixture und erneutem Mandatory-CI auf dem finalen SHA.
   Kein Merge, solange (3) und (4) nicht erfüllt sind.

— END D3-DONE
