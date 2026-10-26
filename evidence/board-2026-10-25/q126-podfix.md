# q126 – demo podman volume-metadata compare fix (coordinator-recorded)

## PLAN
- [x] 1. Mission q126.txt dispatchen (qwen Lane 0311c5b2, 2026-10-26)
- [x] 2. Worker-Artefakt: Commit 6cecb74 in baseharbor-demo, Branch fix/podman-volume-metadata-compare
- [x] 3. Coordinator-Gegenprüfung des Diffs gegen o144-Rezept §4
- [x] 4. Record-Begründung: Worker schrieb die Evidence-Datei nicht persistiert (END-Marker im
      Pane, Datei nirgends auf Platte auffindbar) – dieses Record ersetzt sie; Code-Artefakt
      unabhängig verifiziert. Kein Worker-Commit im Root-Repo.
- [ ] 5. Demo-PR + CI (podman data-capabilities rot->grün, docker bleibt grün)
- [ ] 6. Unabhängiges Review -> Merge-Empfehlung

## Artefakt (verifiziert)
- Repo: baseharbor-demo | Branch: fix/podman-volume-metadata-compare | Commit: 6cecb74 (Parent 9493e82 = main)
- Diff: tests/data-capabilities/new_edit.py +9/-2 (git diff 9493e82..6cecb74, oben ausgelesen)

### Änderung 1 – retained_sql() (Zeile ~222)
Ganz-Dict-Vergleich `volumes(item) != expected_volumes` ersetzt durch
`{_volume_identity(v)}-Mengenvergleich`; `_volume_identity` = (Name, sortierte Labels, CreatedAt).
Namespace/Options (podman-spezifische, über remove instabile Felder) werden nicht mehr verglichen.
Name bleibt im Identity-Tuple -> Zuordnung/Menge bleibt eindeutig geprüft.

### Änderung 2 – foreign-volume guard (Zeile ~361)
Voller Inspect-Dict-Vergleich des foreign-Volumes ersetzt durch denselben Identity-Vergleich;
Container-Id/State/Config-Labels-Guards bleiben unverändert hart.

## Konformität mit o144-Rezept
- §4.2 Wortlaut (Name/Labels/CreatedAt, Namespace/Options normalisiert ignoriert) exakt getroffen.
- Beide von o144 benannten Stellen angepasst; keine weiteren Assertions abgeschwächt.
- Produktlogik (baseharbor) unangetastet – korrekt, da o144 §3a belegt: remove/retention schreibt
  keine Volume-Metadaten.

##/docker-Gate-Hinweis des Workers
q126 berichtete einen docker-Lauf mit exit=1 wegen sudo/rootless-Kontext im Worker-Setup
(Laufzeitumgebung, nicht Code). Da der Diff docker-shaped dicts identisch behandelt (kein
Namespace-Key in docker inspect -> Tuple-Felder unverändert), bleibt docker-Seite semantisch
gleich; maßgebliche Qualifikation ist die Demo-PR-CI selbst (docker + podman Gates).

## Verifikation ausstehend
- Demo-PR-CI: atomic/docker/data-capabilities weiterhin PASS, atomic/podman/data-capabilities
  PASS (statt retained-SQL-Fail), Restmatrix unverändert.
- Danach baseharbor #1077 podman-Job-Rerun gegen neuem Demo-main.
