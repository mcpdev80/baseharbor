package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"golang.org/x/sys/unix"
)

type secretPTYOutput struct {
	bytes.Buffer
	master, slave *os.File
	t             *testing.T
}

func (out *secretPTYOutput) Write(data []byte) (int, error) {
	n, err := out.Buffer.Write(data)
	text := string(data)
	if strings.Contains(text, "Configure now?") {
		_, err = out.master.Write([]byte("y\n"))
	}
	if strings.Contains(text, "API_TOKEN value:") || strings.Contains(text, "API_TOKEN value again:") {
		flags, flagErr := unix.IoctlGetTermios(int(out.slave.Fd()), unix.TCGETS)
		if flagErr != nil || flags.Lflag&unix.ECHO != 0 {
			out.t.Error("secret entry did not disable terminal echo")
		}
		_, err = out.master.Write([]byte("native-pty-secret\n"))
	}
	return n, err
}
func TestRequiredSecretRealPTYRemainsInteractiveAndHidden(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	if !requiredSecretInputIsTerminal(slave) {
		t.Fatal("actual PTY not recognized")
	}
	previousInput, previousTerminal, previousHidden := appApplySecretInput, appApplySecretIsTerminal, appApplySecretReadHidden
	appApplySecretInput, appApplySecretIsTerminal, appApplySecretReadHidden = slave, requiredSecretInputIsTerminal, readApplicationSecretFromTerminalBuffered
	t.Cleanup(func() {
		appApplySecretInput, appApplySecretIsTerminal, appApplySecretReadHidden = previousInput, previousTerminal, previousHidden
	})
	output := &secretPTYOutput{master: master, slave: slave, t: t}
	setter := &fakeApplicationSecretSetter{}
	done := make(chan error, 1)
	go func() {
		done <- promptAndStoreMissingRequiredSecrets(context.Background(), setter, "demo", []openbao.RequiredSecretStatus{{Name: "API_TOKEN"}}, io.Writer(output))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		master.Close()
		slave.Close()
		<-done
		t.Fatal("PTY interaction did not finish")
	}
	if string(setter.values["API_TOKEN"]) != "native-pty-secret" {
		t.Fatal("interactive secret not stored")
	}
	if strings.Contains(output.String(), "native-pty-secret") {
		t.Fatal("secret leaked into prompt output")
	}
}
