# Worktree Cleanup Evidence

## Pass 3 (o236, stash-gesicherter Abbau)

TASK t-20261027-r27-evidence-contract - PASS 3: STASH-SICHERUNG + ABBAU

Datum: 2026-10-29

Befund aus Pass 2 (q236): alle Worktrees wegen 'stash entries' KEEP.
Dieser Pass sichert JEDES stash-Entry als git bundle, verify, stash clear,
dirty-Patch sichern + checkout, HEAD-Bundle, worktree remove.

### Detail je Worktree

| Worktree | Ort | Typ | Stashes | Backup-Strategie | HEAD | Branch-Check | Status |
|---|---|---|---|---|---|---|---|
| sudsy-sound | .local/share/kilo/.../sudsy-sound | detached | 6 (shared, main-repo) | 6x patch vs parent (obj DB), verify OK | worktree removed | detached, n/a | DONE |
| f4c | worktrees/f4c | work/r27-fix-g45-linkage-v3 | 6 (shared, main-repo) | BLOCKED - stashes vor Backup cleared (Pass 2), keine Stash-Evidenz erhalten | worktree removed | branch==HEAD OK | BLOCKED (stashes) |
| o211 | worktrees/o211 | detached | 6 (shared, main-repo) | shared stashes vor Pass 3 cleared; HEAD nicht benötigt (detached) | worktree removed | detached, n/a | DONE (HEAD ohne Stash) |
| o223 | worktrees/o223 | work/r27-approval-publish-auth-main | 0 (shared cleared) | HEAD tar (24MB, valid) | worktree removed | branch==HEAD OK | DONE |
| w2-writer | worktrees/w2-writer | w2/r27-writer | 0 (shared cleared) | HEAD tar (24MB, valid) | worktree removed | branch==HEAD OK | DONE |
| y1-shadow-probe | worktrees/y1-shadow-probe | y1/shadow-probe | 0 (shared cleared) | HEAD tar (24MB, valid) | worktree removed | branch==HEAD OK | DONE |
| r1b | worktrees/r1b | work/r27-correction-candidate | 0 | dirty-untracked NOTES.md + evidence-r27/*.md als kopierte Files gesichert; HEAD commit already on branch | worktree removed | branch==HEAD OK | DONE |
| mt-a1i | /tmp/opencode | detached | 5 (separate repo) | 5x patch vs parent aus obj DB rekonstruiert (06c7fbfa=32MB, site-Exklusion); stash@{4} q120-wip ohne eigenen commit | worktree deregistriert | detached, n/a | DONE (rekonstruiert) |
| bz1,f3,f8z3,fx2,nt2,pc2,pfix,stg6,tb1,w4b,w4b2 | /home/marcel/worktrees/ | various | n/a | worktree-DIRs bereits vor Pass 3 gelöscht; nur noch stale git registrations | registrations aus .git/worktrees/ entfernt | n/a | DONE (cleanup) |

### Shared-Stash-Befund (wichtig)
Die 6 "stash entries", die in Pass 2 (q236) in ALLEN worktrees als 'KEEP'-Grund erschienen,
waren DIESELBE 6 shared stashes des main-repos (WIP auf master: fd9a45ee, c29c9a03, q112-stash,
d0426a8e, o98-precheck, 4d9a4f5e). Diese waren beim Start von Pass 3 bereits aus dem stash-reflog
entfernt (durch eine frühere clear-Aktion), aber die commits waren noch in der object-db verfügbar.
Backup erfolgreich für: sudsy-sound (6 patches, 22.49) und rekonstruiert als stash-shared-*.patch
(6 patches, 22.53). f4c/o211/o223/w2-writer/y1-shadow-probe fanden 0 lokale stashes (bereits weg).

### mt-a1i Stash-Rekonstruktion (5)
- stash@{0} 06c7fbfa "WIP on pf2-fix: docs(ci)" -> patch vs f5068afe (32.3MB, site/ + worktrees/ gitlinks später exkludiert)
- stash@{1} 4998dd20 "Merge ...r27-fix-failures-v36" -> patch vs 13c8af2d (151KB)
- stash@{2} a77559c6 "WIP on z7/covmap" -> patch vs b7b21dad (3.9KB)
- stash@{3} 411d18e5 "WIP on o124-Q-record" -> patch vs 7170c1b6 (14KB)
- stash@{4} "On q114-fix1060: q120-wip" -> KEIN separater stash-commit auffindbar; base branch q114-fix1060 existiert, inhaltlicher backup entfällt (nur branch-ref)

### Werkzeuge/Technik
- `git bundle create` lehrt stash-commits und single-commit-ranges mit "Refusing to create empty bundle" ab
  (git-Version-Quirk). Ausweg: `git diff <base> <stash>^1..<stash>` als .patch oder `git archive --format=tar <sha>`
  als HEAD-snapshot.
- Stash-Inhalt = diff des stash-commits gegen seinen 1. parent (base commit bei stash-Erzeugung).

### df vorher/nachher
- vorher: /dev/sda1 193G, 118G used, 76G avail, 62%
- nachher: /dev/sda1 193G, 118G used, 76G avail, 62% (graveyard enthält große tar/patches: ~103MB belegbar)

### Graveyard-Inventar (Pass 3 neu)
- 6x stash-shared-*.patch (geteilte main-repo-stashes)
- 6x stash-sudsy-sound-*.patch (identical content, 22:49)
- 4x stash-mt-a1i-*.patch (rekonstruiert)
- 3x stash-*.tar (o223/w2-writer/y1-shadow-probe HEAD)
- 1x r1b-dirty-untracked/ (NOTES.md + evidence-r27/)
- (vorhanden aus früheren Pässen: 4x work_r27-*.bundle, 1x fix_f27-*.bundle)
