package main

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func runtimeMutationOwnerBytes(path string, optional bool) ([]byte, error) {
	for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if os.IsNotExist(err) && optional {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, runtimeMutationOwnerError()
		}
	}
	expected, err := os.Lstat(path)
	if os.IsNotExist(err) && optional {
		return nil, nil
	}
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm()&0077 != 0 || expected.Size() > 8<<20 {
		return nil, runtimeMutationOwnerError()
	}
	owner, ok := expected.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) || owner.Nlink != 1 {
		return nil, runtimeMutationOwnerError()
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, runtimeMutationOwnerError()
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(expected, actual) {
		return nil, runtimeMutationOwnerError()
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	current, statErr := os.Lstat(path)
	if err != nil || statErr != nil || len(data) > 8<<20 || !current.Mode().IsRegular() || !os.SameFile(expected, current) || current.Mode().Perm()&0077 != 0 {
		return nil, runtimeMutationOwnerError()
	}
	return data, nil
}
