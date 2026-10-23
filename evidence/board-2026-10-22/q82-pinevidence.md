# Q82-PINEVIDENCE Evidence Job

## PLAN [wip:q82]

1. Erzeuge branch work/v0.4.26-pinevidence ab 1861ed07888a1ec22c218ca491d23ee54f48e261
2. Füge .github/workflows/release-pin-evidence.yml hinzu (workflow_dispatch, inputs: tag/ref/demo_ref/connector_ref, 40-hex-Validierung)
3. Der Workflow führt scripts/ecosystem_release_pins.py aus und lädt docs/releases/<tag>.consumer-pins.json als Artifact
4. Teste Workflow-Mechanismus via dispatch
5. Hänge Run-ID + Artifact-Link an Evidence
6. END Q82-PINEVIDENCE-DONE mit branch-head + run-id

## todoWrite Status

[wip:q82] Evidence-Job für release-pin-evidence implementiert und getest
