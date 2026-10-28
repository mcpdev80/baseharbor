# Shadow-Run Status (coord, 17:1x UTC)
- PreFlight: Sicherheits-Gates OK (KANDIDAT==CONTROLLER_PIN==f4b173d3, Ancestry ✓).
- BLOCKER (authentifiziert, Zeuge: gh api workflows-Liste): receipt-post-run.yml auf GitHub
  UNREGISTERED, weil default branch=main und die Datei nur auf develop existiert.
  Ohne Post-Run wird keine Receipt publiziert -> approval kann keine Receipt genehmigen ->
  shadow-final nicht aufrufbar. Kette fail-closed = korrektes Verhalten.
- Owner-Votum "ohne Post-Run" hatte keine ausführbare Route; dokumentiert statt erzwungen.
- ERLEDIGT statt ShadowLive: 3 Negativtest-Klassen + D3 auf f4b173d3 erneut ausgefuehrt,
  43 Tests rc=0 (Zeuge this file + unittest OK oben).
- NÄCHSTER TRIGGER: main-Promotion des v0.4.27-Zyklus (Release-Freigabe separat!) ->
  Workflow registriert -> Runbook A (4 Owner-Klicks) in ~15 min durchfuehrbar.

## Prozess-Note (Owner 2026-10-28)
Wenn der Controller (receipt_verifier.py u.a.) ein neues Modul erhaelt, muss der
Writer-Pin in receipt-post-run.yml im selben reviewed Change mitwandern — sonst
importiert der Writer Code, den sein Pin nicht kennt (heute: 94f35d02 < Modul).
