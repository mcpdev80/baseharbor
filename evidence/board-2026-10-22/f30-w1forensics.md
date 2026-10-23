# F30 – W1 Run Forensics (READ-ONLY)

Task: Log-Forensik zu W1 run 40009690268 (mcpdev80/baseharbor). Read-only; KEINE Produktänderungen.
Evidence-Pfad: dieses File. Lane-Tag: [wip:f30]

## PLAN

- [ ] 0. Evidence-File + PLAN/todowrite anlegen (dieser Schritt)
- [ ] 1. Run 40009690268 inventarisieren: Jobs, Status, Job-IDs (`gh run view`)
- [ ] 2. Cluster A: private consumer auth – Jobs "Integration remote-target" (docker+podman) + "static/live-console" Log-Forensik
- [ ] 3. Cluster B: wizard-hang + backup-restore-Kaskade – "Reference Journey Full E2E" (beide Runtimes)
- [ ] 4. Cluster C: OpenBao – v0.4.21 "HA data gate" (podman) + "data-capabilities" (beide Runtimes)
- [ ] 5. Repo-Stellen lokalisieren (Datei:Zeile der Fehlstellen) je Cluster
- [ ] 6. Drei Diagnose-Abschnitte in dieses File schreiben (Log-Auszug, Fail-Grund, Root-Cause-Hypothese, Minimal-Fix, keine Umsetzung)
- [ ] 7. Abschluss: END F30-W1FORENSICS-DONE mit 3 clustern

## Cluster A – private consumer auth (Integration remote-target docker+podman; static/live-console)

(inline)

## Cluster B – wizard-hang + backup-restore-Kaskade (Reference Journey Full E2E, beide Runtimes)

(inline)

## Cluster C – OpenBao (v0.4.21 HA data gate podman; data-capabilities beide Runtimes)

(inline)
