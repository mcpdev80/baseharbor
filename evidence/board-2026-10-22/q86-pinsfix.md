# Q86-PINSFIX: Restore missing v0.4.26.consumer-pins.json

## PLAN

- [x] 0. Write this evidence file and initialize todowrite
- [ ] 1. Create v0.4.26.consumer-pins.json with the verified final pins, matching the v0.4.23.consumer-pins.json schema
- [ ] 2. Create branch `work/v0.4.26-restfix` from commit `1861ed07888a1ec22c218ca491d23ee54f48e261` in baseharbor
- [ ] 3. Place `docs/releases/v0.4.26.consumer-pins.json` on that branch
- [ ] 4. Push to origin (NO merge, NO develop)
- [ ] 5. Verify with `scripts/ecosystem_release_pins.py --tag v0.4.26 --core <core>` digest comparison
- [x] 6. END Q86 with branch-head + file-digest

## END Q86

- branch: `work/v0.4.26-restfix`
- branch-head: `abbcc964`
- file-digest: `7bb57274c8c0f178e25f27dd5b16b1eae8c408b664f636547a005e06cfc0ea87`
- SHA-verify: `scripts/ecosystem_release_pins.py --tag v0.4.26 --core 1861ed07888a1ec22c218ca491d23ee54f48e261 --connector df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 --demo 25c319697fd3d89d894943e0f44cd3d5dde8b198` produced matching pins

## Pins

- core: `1861ed07888a1ec22c218ca491d23ee54f48e261`
- demo: `25c319697fd3d89d894943e0f44cd3d5dde8b198`
- console: `69d1d14007fa43dc0c0457c3347280be7f802818`
- connector: `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`

## repo-rule compliance

- Schema: `baseharbor.consumer-evidence-pins/v1` (exact copy from v0.4.23)
- Gates: integration/docker, integration/podman, integration/static/live-console
- Workflow: `.github/workflows/release-integration.yml`
