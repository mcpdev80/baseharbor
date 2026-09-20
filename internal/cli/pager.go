package cli

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

const pagerLineThreshold = 28

func writeMaybePaged(w io.Writer, content string) {
	if !writerIsTerminal(w) || strings.Count(content, "
") < pagerLineThreshold || os.Getenv("TERM") == "dumb" {
		_, _ = io.WriteString(w, content)
		return
	}
	file, ok := w.(*os.File)
	if !ok {
		_, _ = io.WriteString(w, content)
		return
	}
	pager := strings.TrimSpace(os.Getenv("PAGER"))
	if pager == "" {
		pager = "less -FRX"
	}
	parts := strings.Fields(pager)
	if len(parts) == 0 {
		_, _ = io.WriteString(w, content)
		return
	}
	if _, err := exec.LookPath(parts[0]); err != nil {
		_, _ = io.WriteString(w, content)
		return
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = file
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		_, _ = io.WriteString(w, content)
	}
}
