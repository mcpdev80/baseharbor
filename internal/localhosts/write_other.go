//go:build !linux

package localhosts

import "errors"

func ApplySystem(Change) error {
	return errors.New("automatic hosts management currently supports Linux only")
}
