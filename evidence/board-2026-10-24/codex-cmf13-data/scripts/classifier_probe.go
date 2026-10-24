package main
import("errors";"fmt";"io";"strings")
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "network unreachable") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "operation timed out") ||
		strings.Contains(s, io.ErrUnexpectedEOF.Error()) ||
		strings.Contains(s, "reset by peer")
}
func main() { for _, msg := range []string{
"x509: certificate signed by unknown authority; connection reset by peer",
"permission denied: credential file unavailable after connection refused",
"exec bao: executable file not found; i/o timeout",
"connection refused",
"permission denied: temporary credentials file",
} { fmt.Printf("%q => retry=%v\n", msg, isTransientError(errors.New(msg))) } }
