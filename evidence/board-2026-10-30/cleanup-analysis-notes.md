Jetzt habe ich alle Daten. Hier ist die Analyse:

---

## Analyse: baseharbor-ecosystem Bereinigungspotenzial

### 1. Gesamtübersicht

| Metrik | Wert |
|--------|------|
| **Worktrees gesamt** | 86 |
| **Disk-Größe Worktrees** | 2.1 GB |
| **Disk-Größe evidence/** | 385 MB |
| **Disk-Größe tmp-ops/** | 900 KB |
| **Disk-Größe evidence-r27/** | 108 KB |
| **Disk-Größe wt-wizardfix/** | 27 MB |
| **Disk-Größe scripts/** | 2.5 MB |
| **Ahead von Origin** | 1638 Commits |

---

### 2. Müll / Alte Worktrees (Kandidaten für Bereinigung)

#### A. Veraltete v0.4.26 Release Worktrees (6 Worktrees)
Diese beziehen sich auf ein bereits published Release und sind archiviert:

| Worktree | Branch | Commit |
|----------|--------|--------|
| `demo-integrated-1022` | chore/v0426-core-pin | a44e2e9 |
| `f49-demorepin` | work/v0.4.26-demo-repin | f57c8b6 |
| `q110-openbao` | work/v0.4.26-openbao-gateplan | 7a3c4520 |
| `q113-1052` | work/v0.4.26-openbao-retry | 9f715184 |
| `q123-rev1052` | (detached) | 9f715184 |
| `o140-policy` | work/v0.4.26-ci-qualifier-1060 | 54dfa458 |

**`q113-1052` und `q123-rev1052` sind Duplicates** (gleicher SHA 9f715184).

#### B. Veraltete CI-Infra / Podman Worktrees (4 Worktrees)
Aus Oktober 2024, vor der R27-Migration, nicht mehr relevant:

| Worktree | Branch | Commit |
|----------|--------|--------|
| `podman-acceptance-typed-consent` | (detached) | 78a72eba |
| `scoped-runtime-reset-xdg-992` | (detached) | b981f3b6 |
| `core-754-mcp-precondition-classification` | fix/core-754-... | e1c797bb |
| `core-921-central-cli-mcp-registration` | runtime-validation/... | e577af94 |

#### C. q-Serie Duplicates (4 Worktrees)
Zwei Paare mit identischem SHA:

| Worktree 1 | Worktree 2 | SHA |
|------------|------------|-----|
| `q8-sim` | `q9-record` | 3a0df114 |
| `q113-1052` | `q123-rev1052` | 9f715184 |
| `nq2` | `stg8b` | 3f794684 |
| `b3-ustrack` | `l7-ustrack` | 8f8e5547 |

---

### 3. Worktrees die NICHT auf master gemerged sind

Diese haben noch unveröffentlichten Code:

| Worktree | Branch | Status |
|----------|--------|--------|
| `bz2` | work/r27-belegsetz2 | detached HEAD |
| `bz3` | work/r27-belegsetz3 | detached HEAD |
| `doc1` | work/r27-evidence-index | NOT MERGED |
| `demo-integrated-1022` | chore/v0426-core-pin | NOT MERGED |
| `f25-console2` | f25/console2-live-workflow-proofs | NOT MERGED |
| `f49-demorepin` | work/v0.4.26-demo-repin | NOT MERGED |
| `f66-testfix` | fix/podman-volume-metadata-compare | NOT MERGED |
| `nc3-f26-acceptance` | f26-nc3-acceptance | NOT MERGED |
| `pl2` | work/r27-board-sync2 | NOT MERGED |
| `pl3` | work/r27-board-sync3 | NOT MERGED |

---

### 4. l-Serie ustrack (9 Worktrees)
Sehr ähnlich – fast identische Commits, möglicherweise redundant:

| Worktree | SHA | Commit Message |
|----------|-----|-----------------|
| `l1-ustrack` | 9b185fd2 | move post-run dormancy guard |
| `l2-ustrack` | 818f4889 | correct approval workflow dormancy |
| `l3-ustrack` | 8fcf083f | pin workflow-policy expectations |
| `l4-ustrack` | cb01d090 | sync expected allowlist |
| `l5-ustrack` | 6e170894 | bounded automatic task link |
| `l6-ustrack` | 83d53e86 | exact path mapping |
| `l7-ustrack` | 8f8e5547 | map ci-workflow-policy.sh |
| `r2-ustrack` | c1c7db7b | resolve real base SHA |
| `b3-ustrack` | 8f8e5547 | map ci-workflow-policy.sh (DUPLICATE!) |

**`b3-ustrack` und `l7-ustrack` haben denselben SHA** – `b3-ustrack` ist ein Duplikat.

---

### 5. Topo und Promote (3 Worktrees)
| Worktree | Branch | SHA |
|----------|--------|-----|
| `topo` | work/r27-tuple4-pinbump | c7a58b20 |
| `prom` | work/promote-r27-main2 | 7a3437ea |
| `prm6` | work/promote-r27-main6 | 2033158c |

Diese sind vermutlich Arbeitsschritte für die R27 Promotion, aber `prom` und `prm6` könnten redundant sein.

---

### 6. Bereinigungsvorschläge

#### Sofort entfernbar (ca. 150 MB):
| Kategorie | Worktrees | Anzahl |
|-----------|-----------|--------|
| Alte v0.4.26 Releases | `demo-integrated-1022`, `f49-demorepin`, `q110-openbao`, `q113-1052`, `q123-rev1052`, `o140-policy` | 6 |
| Alte Podman/CI-Infra | `podman-acceptance-typed-consent`, `scoped-runtime-reset-xdg-992`, `core-754-...`, `core-921-...` | 4 |
| Duplicate Worktrees | `q8-sim`/`q9-record`, `b3-ustrack`/`l7-ustrack`, `nq2`/`stg8b`, `q113-1052`/`q123-rev1052` | 4 (halbe Paare) |
| Leere/leere Branch Worktrees | `a2-accept`, `ci-core921-coverage-binding`, `f61-r5` bis `f66-testfix` (7 Worktrees) | 7 |

#### Insgesamt: ~21 Worktrees entfernbar, ca. 150-200 MB

#### Bedenkenswert:
| Worktree | Grund |
|----------|-------|
| `o142-consentfix` | Standalone, kein Branch |
| `o51b-1042-sync` | Standalone |
| `pin1` | Standalone, v3.9-final |
| `o211`, `o226`, `o233` | Neuere Standalone-Worktrees (29. Okt) – noch relevant? |
| `f25-console2` | Nicht gemerged, aber veraltet |
| `nc3-f26-acceptance` | Nicht gemerged, veraltet |
| `pl2`, `pl3` | Board-Sync, nicht gemerged |

#### Struktur-Vorschlag für Worktrees:
1. **Archiv** → Alte Releases und Podman-Infra
2. **r27-active** → Nur aktive R27-Worktrees (f4c, f8z, hsync, w2-writer, w3-verifier, w8-plansync)
3. **o-Serie** → Neuere o2xx Worktrees (möglicherweise noch relevant)

---

### 7. Zusätzliche Unordnung

| Bereich | Problem |
|---------|---------|
| `worktrees/` | 86 Worktrees, 2.1 GB – massive Ansammlung |
| `evidence/` | 385 MB, 17+ Unterverzeichnisse – historisch gewachsen |
| `tmp-ops/` | 900 KB – vier Scratch-Verzeichnisse |
| `evidence-r27/` | 108 KB – 16 markdown files für R27 Evidence |
| `wt-wizardfix/` | 27 MB – eigenes Sub-Repo |
| `baseharbor/` | 1 untracked Change, Submodule-Status unklar |
| `baseharbor-demo/` | 6 untracked Changes |
| `baseharbor-console/` | 2 untracked Changes |

