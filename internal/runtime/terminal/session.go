// Package terminal defines the bounded terminal transport supplied by runtimes.
package terminal

import (
	"context"
	"errors"
	"io"
)

type Session interface {
	io.ReadWriteCloser
	Resize(rows, columns int) error
	Wait(context.Context) (int, error)
}

func ValidateSize(rows, columns int) error {
	if rows < 1 || rows > 512 || columns < 1 || columns > 512 {
		return errors.New("terminal rows and columns must be between 1 and 512")
	}
	return nil
}
