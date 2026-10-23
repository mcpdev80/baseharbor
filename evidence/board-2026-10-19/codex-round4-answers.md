1. **Q1 — Use a fresh GitHub-hosted Docker runner for the focused proof.** “Docker only” specifies the runtime; `baha-docker` is not an acceptance requirement. Keep `assertCorelessHost` intact and leave both shared stacks untouched.

   The local precedent for hosted native Docker execution is `.github/workflows/docker-runtime-acceptance.yml`, job `provider-ha` (`ubuntu-24.04`). Use that hosting pattern for a dedicated focused job, with:

   ```bash
   BASEHARBOR_TEST_RUNTIME=docker \
   BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true \
   go test ./cmd/baha \
     -run '^TestAppEditMCPAppEditRunsRealProviderEditLifecycle$' \
     -count=1 -v -timeout=35m
   ```

   Require isolated state/configuration, the exact reviewed source SHA, an executed **PASS**, and retained logs identifying the runner, Docker inventory, source and outcome. A dedicated disposable VM with its own Docker daemon is also technically acceptable; a container mounting either shared host’s Docker socket provides insufficient isolation.

   Correction to the question’s premise: `o10-native-mcp-proof.md` records execution on **`baha-docker`**, not GitHub-hosted CI. It also tested the earlier `baseharbor.apply` path, so it cannot establish the corrected `app.edit` behavior.

2. **Q2 — Use `docs/internal/ci-binding-qualifications.md` as the reviewed policy document, containing exactly one fenced JSON record.** Adapt the loader to read that exact path exclusively from the authenticated target-base revision and strictly extract that single record. Missing, ambiguous or malformed content must fail closed.

   This uses the existing Markdown documentation classification in both the current candidate and locally available `origin/develop`. Moving JSON to `policy/` or `.ci-policy/` would merely hit the unknown-path blocker. Do **not** add a general policy-directory exemption or exempt the original `scripts/ci-*` path.

   The sequence remains: finish and independently review the loader adjustment → freeze the new H → independently review and merge the record-only prerequisite with mandatory green CI → qualify H against that trusted base. Validate the record’s complete inventory, hashes and check IDs before publication. Documentation classification supplies the bootstrap route; it does not establish independent qualification.

   This is a narrowly scoped design adjustment to #1007, invalidating its previous frozen SHA and hashes. It does not reopen the exhausted correction budget or authorize implementation under this read-only request.

3. **Q3 — No mechanism acceptance yet.** My bounded source inspection found reasons not to adopt Q15’s blanket approval:

   - `_valid_binding_record` accepts incomplete hash coverage: two entries in `files` and only one in `file_hashes` pass validation. The content verifier then checks only the supplied hashes. Require identical key sets.
   - `repository` and `target_branch` are required fields but never compared with the authenticated evaluation context.
   - `plan(..., repository=...)` omits that repository when calling the record loader.
   - Moving a repro into ecosystem evidence does **not** place the qualification record on the authenticated product target base, as Q15 claims.

   These need a focused enforcement review before acceptance, together with the bootstrap adjustment. Also, R8’s claim that `DOC_CHECKS` does not exist is incorrect at `e2060ea0`; I verified that it exists. Its three reported **CI_CHECKS IDs** are correct.

All verification was local and read-only. No files, GitHub state or runtime resources were changed.

RULING-4-DONE
