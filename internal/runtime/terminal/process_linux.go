package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type processSession struct {
	file      *os.File
	ctx       context.Context
	cancel    context.CancelFunc
	cmd       *exec.Cmd
	done      chan struct{}
	closeOnce sync.Once
	exitCode  int
}

// Start runs a runtime-owned command with an actual PTY. Callers select a typed
// container operation; this primitive is never exposed as a machine operation.
func Start(ctx context.Context, cmd *exec.Cmd, rows, columns int) (Session, error) {
	if err := ValidateSize(rows, columns); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(columns)})
	if err != nil {
		return nil, errors.New("runtime terminal could not start")
	}
	// Nonblocking mode enables os.File deadlines and wakes blocked I/O on Close.
	if err := syscall.SetNonblock(int(file.Fd()), true); err != nil {
		_ = cmd.Process.Kill()
		_ = file.Close()
		_ = cmd.Wait()
		return nil, errors.New("runtime terminal cannot support bounded I/O")
	}
	lifetime, cancel := context.WithCancel(ctx)
	s := &processSession{file: file, ctx: lifetime, cancel: cancel, cmd: cmd, done: make(chan struct{})}
	context.AfterFunc(lifetime, func() { _ = s.Close() })
	go func() {
		err := cmd.Wait()
		s.exitCode = 0
		if err != nil {
			s.exitCode = cmd.ProcessState.ExitCode()
		}
		close(s.done)
	}()
	return s, nil
}

func (s *processSession) Read(data []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	return s.file.Read(data)
}
func (s *processSession) Write(data []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	return s.file.Write(data)
}
func (s *processSession) Resize(rows, columns int) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := ValidateSize(rows, columns); err != nil {
		return err
	}
	return pty.Setsize(s.file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(columns)})
}
func (s *processSession) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.done:
		return s.exitCode, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}
func (s *processSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		select {
		case <-s.done:
		default:
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		err = s.file.Close()
	})
	return err
}
