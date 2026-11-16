package externalprovider

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"strings"
)

// Private snapshots stay inside adapters; no material digest enters a public
// registration, result or manifest. The existing local reader remains authority.
type PreparedMaterial struct {
	reference    string
	confidential bool
	owner        os.FileInfo
	data         []byte
}

func PrepareMaterialReference(reference string, confidential bool) (*PreparedMaterial, error) {
	reference = strings.TrimSpace(reference)
	data, err := ReadMaterialReference(reference, confidential)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(reference)
	if err != nil {
		zeroMaterial(data)
		return nil, errors.New("external material reference is invalid")
	}
	owner, err := os.Lstat(u.Path)
	if err != nil {
		zeroMaterial(data)
		return nil, errors.New("external material owner changed during preparation")
	}
	prepared := &PreparedMaterial{reference: reference, confidential: confidential, owner: owner, data: data}
	if err := prepared.Revalidate(); err != nil {
		zeroMaterial(data)
		return nil, err
	}
	return prepared, nil
}
func (p *PreparedMaterial) Bytes() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.data...)
}
func (p *PreparedMaterial) Revalidate() error {
	if p == nil {
		return nil
	}
	u, err := url.Parse(p.reference)
	if err != nil {
		return errors.New("external material reference changed")
	}
	owner, err := os.Lstat(u.Path)
	if err != nil || !os.SameFile(p.owner, owner) {
		return errors.New("external material owner changed; replan before delivery")
	}
	data, err := ReadMaterialReference(p.reference, p.confidential)
	if err != nil {
		return err
	}
	defer zeroMaterial(data)
	after, err := os.Lstat(u.Path)
	if err != nil || !os.SameFile(p.owner, after) || !bytes.Equal(p.data, data) {
		return errors.New("external material changed; replan before delivery")
	}
	return nil
}
