package main
import("errors";"fmt";"io";"strings")
func permanentErrorCause(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "x509"),
		strings.Contains(s, "handshake"),
		strings.Contains(s, "certificate"):
		return "tls"
	case strings.Contains(s, "permission denied"),
		strings.Contains(s, "permission"):
		return "permission"
	case strings.Contains(s, "executable file not found"),
		strings.Contains(s, "is not executable"),
		strings.Contains(s, "no such file or directory"),
		strings.Contains(s, "exec "):
		return "exec"
	default:
		return ""
	}
}
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if permanentErrorCause(err) != "" {
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
func main(){ for _, msg := range []string{
"connection refused",
"compose exec -T openbao-member-1 sh -c bao status -format=json: connection refused",
"compose exec -T openbao-member-1 sh -c bao status -format=json: connection reset by peer",
"x509: certificate signed by unknown authority; connection reset by peer",
"permission denied: credential file unavailable after connection refused",
"exec bao: executable file not found; i/o timeout",
} { fmt.Printf("%q => permanent=%q retry=%v\n",msg,permanentErrorCause(errors.New(msg)),isTransientError(errors.New(msg))) } }
