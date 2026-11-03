# w59 candidate-contract r5 — fix-5 evidence

ROLLE: Worker (Lane w59). Coord f49a3303. KARTE t-20261018-r27-candidate-contract (runde 5).
REVIEWER/GRUNDLAGE: review-1355d (runde 4, Urteil REJECT). SCOPE IST ERLEDIGT:
Coordinator-Scope-Vermerk 2026-11-03 publiziert (cmd/baha consumer + internal/deployment
Consent-Feld + CLI/Spec-Index-Zeilen freigegeben).
KANDIDAT: PR #1355, branch work/r27-candidate-contract, HEAD 3b0fd03d9c5cc5374dd3c5960faacc2aa1d3b2bf
(== origin ls-remote). develop 0107d8e1 ist Vorfahr.

## END-ZUSTAND
END w59-candidate-e: head=3b0fd03d digest=hart prompt=bindend schema=scharf links=echte-pfade

## PLAN (numerierte Schritte)
1. fetch origin + baseharbor auf work/r27-candidate-contract (HEAD 3b0fd03d, == ls-remote). DONE
2. review-1355d + Scope-Vermerk lesen; Scope (cmd/baha consumer, internal/deployment consent, index lines) = ERLEDIGT. DONE
3. Punkt 1 verifizieren: finalizeCandidateConsent speichert nur den berechneten Digest, raw wird nie persistiert; Validate vergleicht raw==errechnet hart (constant time). DONE (code-les_verified)
4. Punkt 2 verifizieren: promptConfirmCandidateSubmission zeigt canonical digest uber denselben compute-Pfad (CandidateConsentDigest). DONE (code-les_verified)
5. Punkt 3 verifizieren: schema==Go Paritat (const baseharbor.candidate/v1, DisallowUnknownFields im testmapper, ein Vektor-Set fur beide Validierer, SecretRef am echten Owner-Feld). DONE (code-les_verified)
6. Punkt 4 beheben: Decision-Links in docs/spec + docs/de/spec candidate-contract zeigen auf baseharbor-plan/docs/decisions (falsch); echter Ziel-Pfad baseharbor-plan/decisions/<datei>.md (kein docs/ Praefix). FIXED + verifiziert.
7. gofmt/vet leer + go build ./... + go test ./internal/delivery ./internal/application ./contracts + cmd/baha -run Candidate|Consent|Apply gruen; 0 geloeschte Tests ('^-func Test' leer). DONE.
8. Evidence fix-5.md schreiben (je Punkt Datei:stelle + Testname).
9. push FF + ls-remote==head + END an f49a3303.

## repo-rule compliance
- Diff '^-func Test' leer (keine bestehende testfunction geloescht) — VERIFIZIERT: 0 deletions, 126 additions.
- gofmt/vet leer — VERIFIZIERT: gofmt -l leer, go vet rc=0.
- GOCACHE=/tmp/opencode/gc TMPDIR=/tmp; go build ./... rc=0.
- go test ./internal/delivery ./internal/application ./contracts -count=1 -timeout 8m: PASS (rc=0).
- go test ./cmd/baha -run 'Candidate|Consent|Apply' -count=1 -timeout 8m: PASS (rc=0).

## Befund je Punkt (Datei:stelle + Test)
Punkt 1 (digest hart, raw nie persistiert):
- cmd/baha/app_candidate_consent.go:73-89 finalizeCandidateConsent — speichert Digest = CandidateConsentDigest(submission) (L80), NIE rawDigest; empty raw => nil (L74-76).
- cmd/baha/deployment_record.go:117-127 — consent nur aus finalizeCandidateConsent; Validate(consent) verlangt digest==errechnet; consent==nil => errConsentRequiredForCandidateSubmission (L122).
- internal/delivery/candidate.go:129-136 Validate — constantTimeEqual(consent.Digest, CandidateConsentDigest(c)); mismatch => err; kein write ohne Ubereinstimmung.
- Gegen-Test (64-hex falsch): TestCandidateConsentDigestMustMatch (internal/delivery/candidate_test.go:87) legt wrong=&CandidateConsent{Digest:"deadbeef"} an => rejected.

Punkt 2 (prompt zeigt gebundenen digest, ein compute-pfad):
- cmd/baha/app_candidate_consent.go:42-64 promptConfirmCandidateSubmission baut submission (L46-53) + digest = CandidateConsentDigest (L54).
- cmd/baha/deployment_record.go:106-113 write-bau identische Felder (ContractVersion/ApplicationID/TargetName/SourceRepository/SourceRevision/ImageDigest="") + CandidateConsentDigest (via finalize L80).
- EIN compute-pfad: beide Stellen rufen delivery.CandidateConsentDigest(submission) auf. STOP-Kriterium (zwei hash-pfade) NICHT vorhanden.
- cmd/baha/app_apply.go:160-162 + app_remote_lifecycle.go:138-141 rufen promptConfirmCandidateSubmission + recordDeploymentBeforeMutation auf.

Punkt 3 (schema==Go scharf):
- contracts/candidate/v1/candidate-contract.schema.json: additionalProperties:false; source.revision minLength:1 (Go seiten: isPinnedRevision ^[0-9a-f]{40}$|^[0-9a-f]{64}$|^[a-z0-9]+:[0-9a-f]{32,}$).
- contracts/candidate_contract_test.go:37-89 toSubmission (ein mapper) validiert BOTH golden (L152) + negative (L171) gegen Go leaf (Validate) UND published schema (validateAgainstSchema) — ein Vektor-set fur beide validierer.
- DisallowUnknownFields: toSubmission json.Unmarshal in map[string]any + typed struct; schema additionalProperties:false fängt unknownKey (TestCandidateSchemaForbidsUnknownKeys, L244).
- SecretRef am echten owner-feld: TestCandidateRejectsBareSecretReference (L233) + golden/negative fixtures source.secretRef; Go: candidate.go:147-149 reject.

Punkt 4 (echte决策-pfade, kein docs/ praefix):
- baseharbor-plan ls: decisions/2026-10-18-r27-native-delivery-and-stages.md (kein docs/).
- FIX docs/spec/candidate-contract-v1.md:98,107 + docs/de/spec/candidate-contract-v1.md:111 — baseharbor-plan/docs/decisions/ => baseharbor-plan/decisions/.
- Gegen-Test: grep 'docs/decisions' docs/ => leer nach fix; Ziel-URL zeigt auf echte ziel-datei.

## Endgültiger Zustand
digest=hart (raw wird nie persistiert, Validate vergleich hart) | prompt=bindend (ein compute-pfad, CandidateConsentDigest) | schema=scharf (additionalProperties + Go parity + SecretRef am owner) | links=echte-pfade (baseharbor-plan/decisions/, kein docs/)
