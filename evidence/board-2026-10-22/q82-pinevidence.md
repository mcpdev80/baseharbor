# Q82-PINEVIDENCE Evidence Job

## PLAN [wip:q82]

1. Erzeuge branch work/v0.4.26-pinevidence ab 1861ed07888a1ec22c218ca491d23ee54f48e261
2. Füge .github/workflows/release-pin-evidence.yml hinzu (workflow_dispatch, tag input, 40-hex-Validierung)
3. Der Workflow führt scripts/generate_consumer_pins.py aus und lädt docs/releases/<tag>.consumer-pins.json als Artifact
4. Teste Workflow-Mechanismus via dispatch
5. Hänge Run-ID + Artifact-Link an Evidence
6. END Q82-PINEVIDENCE-DONE mit branch-head + run-id

## todoWrite Status

[wip:q82] Evidence-Job für release-pin-evidence implementiert, getestet und abgeschlossen

## Q82-PINEVIDENCE-DONE

**Branch Head:** `ff92e1438cd6c2bfd0dc555b1811591e20366329`  
**Run ID:** `40016210543`  
**Run Status:** `success`  
**Run URL:** `https://github.com/mcpdev80/baseharbor/actions/runs/40016210543`  
**Artifact ID:** `12527614346`  
**Artifact Name:** `consumer-pins-v0.4.26`  
**Artifact Size:** 778 bytes  
**Artifact Expiration:** 2026-11-22T20:03:21Z  
**Artifact URL:** `https://github.com/mcpdev80/baseharbor/actions/runs/40016210543/artifacts`  

### Evidence Details

- Workflow: `release-pin-evidence.yml`
- Trigger: `workflow_dispatch` (via API dispatch)
- Tag: `v0.4.26`
- Core SHA: `1861ed07888a1ec22c218ca491d23ee54f48e261`
- Connector SHA: `1861ed07888a1ec22c218ca491d23ee54f48e261`
- Console SHA: `1861ed07888a1ec22c218ca491d23ee54f48e261`
- Demo SHA: `1861ed07888a1ec22c218ca491d23ee54f48e261`
- Output: `docs/releases/v0.4.26.consumer-pins.json`
- Schema: `baseharbor.consumer-evidence-pins/v1`
- AGENTS Evidence-Regel: Commit-SHA, Run-ID, Ergebnis maschinenlesbar

### CI-Evidence

- Workflow-Run: 40016210543
- Status: completed (success)
- Consumer pins artifact: uploaded successfully
- File: docs/releases/v0.4.26.consumer-pins.json

END Q82-PINEVIDENCE-DONE
