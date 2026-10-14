package externalprovider

import (
	"encoding/json"
	"errors"
	"strings"
)

type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

func ResolveCredentialReference(reference string) (Credentials, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Credentials{}, nil
	}
	data, err := ReadMaterialReference(reference, true)
	if err != nil {
		return Credentials{}, err
	}
	defer zeroMaterial(data)
	return DecodeCredentialMaterial(data)
}

// Decode captured protected input without a second read of mutable source files.
func DecodeCredentialMaterial(data []byte) (Credentials, error) {
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, errors.New("external credential reference contains invalid supported credential JSON")
	}
	if creds.Username == "" && creds.Password == "" && creds.Token == "" {
		return Credentials{}, errors.New("credential reference contains no supported credentials")
	}
	return creds, nil
}
