# #1077 stale-impact-consent: Root-Cause-Analyse (lokale Docker-Reproduktion, 2026-10-25)

Environment: Host mit rootless docker (nach Reboot: uidmap-Package fehlte, dann Behobs),
Gate wie CI: acceptance.sh data-capabilities, Demo-main 9493e82 (#125), Candidate 87736a3647 (#1077 head).
Reproduktion DETERMINISTISCH: Journey `new-edit-new-sql-add` failt in jedem Lokallauf mit
`provider change: stale-impact-consent` (6/6 Laeufe). `remove`/`cancel`/`no` gruen.

## Nachgewiesener Mechanismus (Digest-Ebene)

Consent-Plan (wizard.json == apply.json PlanID, z.B. e13a5c377fc7) vs Revalidierungs-Plan
(digest-6f12d7b86acd) unterscheiden sich in genau 5 Feldern:
  /Facts[postgresql/application/<app>/dev-*/default-*/State: unverifiable -> stopped-but-owned
  /Consumers[.../dev/Requirements[0]/State:    unverifiable -> stopped-but-owned
  /Plan/impacts[0]/before/state:               unverifiable -> stopped-but-owned
  /Plan/impacts[0]/operation:                  add          -> resume
  /Plan/revision

Ursache: observeNativeBackendProviderFacts (cmd/baha/application_native_backend_observation.go)
hat 6 lautlose Early-Returns. Wenn nativeBackendInventory mit ENOENT auf der App-State-Runtime
fehlschlaegt (`lstat .../deployments/<id>/state/demo/runtime: no such file or directory` -
observe.log, 100% der Unverifiable-Faelle), fehlt der Fact komplett -> Snapshot setzt
State=unverifiable, Plan=operation add. Die naechste Observation (wenige 100ms spaeter, nach
first-touch Read des Apply-Pfads, der das Verzeichnis selbst anlegt/sichtbar macht) liefert
count=0 -> stopped-but-owned -> operation resume -> PlanID-Aenderung ->
appMachineChangeDesired :428 fail-closed stale-impact-consent.

Zusaetzlich (gate4-6, seltener): einzelner ListRuntimeContainers-Durchlauf ließ laufenden
Container fehlen (kein die/start-Event!) -> Phantom-stop -> derselbe Flip. Dafuer liegt ein
fertiger, getesteter Patch im Arbeitszweig: observeCanonicalProviderContainersConfirmed
(zweit-Listung vor stopped-Urteil, 6 Call-Sites, Unit-Test TestZeroContainerListingIsConfirmedByFreshListing).
Er beseitigt den Flip NICHT allein, weil die Drift-Hauptursache der ENOENT-Inventory-Race ist.

## Offene Design-Entscheidung (NICHT getroffen)

Consent-Digest (internal/application/provider_change_digest.go) normalisiert bereits
Sampling-Rausch (RAM/Containers, RemainingValiditySeconds), aber NICHT den Uebergang
unverifiable -> beobachtetes-retained (gleiche Instanz, gleiches Intent, hoehere Evidenz).
Optionen:
  A) Digest behandelt unverifiable als Aequivalent zu jedem spaeter beobachteten Zustand
     derselben retained-Instanz (Consent-Zusage "unter Unsicherheit" deckt Aufloesung ab;
     ConfirmationsRequired=2 bleibt).
  B) observeNativeBackendProviderFacts unterscheidet ENOENT-retained (beweisbar aus
     Binding-/Intent-Lage -> direkt stopped-but-owned) von echten API-Fehlern (unverifiable).
     => Preview und Revalidierung sehen von Anfang an denselben Zustand; PlanID stabil.
  C) Apply-Pfad: Re-Proposal in appMachineChangeDesired muss Dieselbe Read-Instanz wie
     :247/:252 verwenden (Shared-Observation wie von da4d919b intendiert, aber nach
     changeDesired durchgereicht statt neu observiert).
Empfehlung: B (+C als Haertung); A verwässert die Consent-Semantik.

## Lokale Artefakte

- Instrumentierung NUR lokal im Worktree worktrees/m1077 (zz_debug_*.go, debugDumpPlan in
  app_edit.go/app_edit_apply.go, observe.log-Hooks in den 2 observation-Dateien, Digest-Dump in
  provider_change_plan.go, debugLogConsent in provider_change_consent.go). VOR einem Commit entfernen.
- Der Confirmed-Listing-Patch (application_provider_container_observation.go + Test + 6 Call-Sites)
  ist committbar und bleibt sinnvoll.
- Ops-Work: /home/marcel/tmp-ops/{bin-m1077,rtimg,demo-run,plandump}, Logs m1077-gate*.log.

END M1077-CONSENT-DRIFT-ANALYSIS
