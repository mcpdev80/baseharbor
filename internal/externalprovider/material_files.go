package externalprovider

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxMaterialBytes = 1 << 20

// ReadMaterialReference resolves the supported local adapter without mutating
// the source. Public trust may be readable; private credentials need one owner.
func ReadMaterialReference(reference string, confidential bool) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || u.User != nil || u.Host != "" || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Scheme != "" && u.Scheme != "file" || !filepath.IsAbs(u.Path) || strings.ContainsAny(u.Path, "\r\n\x00") {
		return nil, errors.New("external material requires a supported absolute local file reference")
	}
	path := filepath.Clean(u.Path)
	if checkStoreParents(path) != nil {
		return nil, errors.New("external material parent is redirected or unavailable")
	}
	before, err := os.Lstat(path)
	if err != nil || !validMaterialOwner(before, confidential) || before.Size() > maxMaterialBytes {
		return nil, errors.New("external material requires bounded regular input with its required owner protection")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("external material cannot be securely opened")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !validMaterialOwner(opened, confidential) {
		return nil, errors.New("external material changed during inspection")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxMaterialBytes+1))
	after, statErr := os.Lstat(path)
	if readErr != nil || statErr != nil || !os.SameFile(opened, after) || !validMaterialOwner(after, confidential) || !opened.ModTime().Equal(after.ModTime()) || opened.Size() != after.Size() || len(data) > maxMaterialBytes || checkStoreParents(path) != nil {
		zeroMaterial(data)
		return nil, errors.New("external material changed during secure read")
	}
	return data, nil
}

func validMaterialOwner(info os.FileInfo, confidential bool) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode().Perm()&0022 == 0 && (!confidential || info.Mode().Perm()&0077 == 0 && checkStoreOwner(info) == nil)
}

func zeroMaterial(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
