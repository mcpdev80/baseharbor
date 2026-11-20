package deployment

import (
	"strconv"
	"strings"
)

// This translation unit holds the deployment record version-mismatch helpers
// that keep deployment_store.go under the 800-line readability ceiling enforced
// by scripts/source-readability-audit.sh in CI. The version gate raises a
// deployable_version_mismatch DeploymentRecordStateError when an on-disk record
// was written by an older shipped BaseHarbor; these helpers extract the exact
// found/supported versions so the CLI can surface them instead of the masked
// "no registered deployment" the final warning loops previously printed.

// DeploymentRecordVersionMismatchVersions returns the record version the
// deployment.json actually carried and the version this binary supports for a
// deployable-version-mismatch error's inner message. It parses the two
// integers the store's version gate embeds in its message so the CLI can
// surface the exact found/expected versions; a parse miss yields (0,0) and the
// message falls back to the stored error text.
func DeploymentRecordVersionMismatchVersions(innerErr error) (found, expected int) {
	return parseTwoVersionNumbers(innerErr)
}

// parseTwoVersionNumbers extracts the first two standalone integers of a
// message. The deployment record version gate embeds "found is not the
// supported expected", so the first integer is the found record version and
// the second the supported version. Punctuation glued to a number (the trailing
// colon after the second version) is stripped before numeric parsing so it is
// not mistaken for a non-numeric token.
func parseTwoVersionNumbers(err error) (found, expected int) {
	tokens := strings.Fields(err.Error())
	found, expected = 0, 0
	for _, tok := range tokens {
		n := extractLeadingInteger(tok)
		if n == 0 {
			continue
		}
		if found == 0 {
			found = n
			continue
		}
		expected = n
		break
	}
	return found, expected
}

// extractLeadingInteger parses the first integer token, tolerating trailing
// punctuation (for example "2:" from "... version 2:") that a naive Atoi would
// reject.
func extractLeadingInteger(tok string) int {
	n, err := strconv.Atoi(strings.TrimRight(tok, ":;,"))
	if err != nil {
		return 0
	}
	return n
}
