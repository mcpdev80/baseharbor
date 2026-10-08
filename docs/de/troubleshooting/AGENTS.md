# Regeln für die Fehlerbehebung von Agenten

Vor der Untersuchung eines Docker, Podman, TLS, Identität, Anbieter oder Akzeptanzausfall:

1. Suchen `.github/known-failures.yaml` für die beobachtete Fehlersignatur.
2. Lesen Sie den verlinkten Vorfall unter `docs/troubleshooting/incidents/`.
3. Überprüfen Sie, ob die dokumentierten Fix-Commits im aktuellen Zweig vorhanden sind.
4. Wiederverwendung des dokumentierten Regressionstests vor Beginn des neuen GitHub CI.
5. Wiederholen Sie ein bekanntes CI-Experiment nicht, es sei denn, die vorhandenen Beweise sind veraltet oder der entsprechende Code wurde geändert.
6. Wenn ein neuer wiederkehrender Fehler behoben wird, fügen Sie sowohl den Registryeintrag als auch sein Ereignisdokument hinzu oder aktualisieren Sie ihn.
