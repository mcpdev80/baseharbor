package applicationbackup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	defaultArgonTime    = 3
	defaultArgonMemory  = 64 * 1024
	defaultArgonThreads = 4
	argonKeyLength      = 32
	saltLength          = 16
)

type KDFParams struct {
	Algorithm string `json:"algorithm"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
	KeyLength uint32 `json:"key_length"`
	Salt      string `json:"salt"`
}

type Envelope struct {
	Magic      string    `json:"magic"`
	Version    int       `json:"version"`
	KDF        KDFParams `json:"kdf"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
}

type envelopeAAD struct {
	Magic   string    `json:"magic"`
	Version int       `json:"version"`
	KDF     KDFParams `json:"kdf"`
}

func newKDFParams() (KDFParams, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return KDFParams{}, fmt.Errorf("generate backup salt: %w", err)
	}
	return KDFParams{
		Algorithm: "argon2id",
		Time:      defaultArgonTime,
		MemoryKiB: defaultArgonMemory,
		Threads:   defaultArgonThreads,
		KeyLength: argonKeyLength,
		Salt:      base64.RawStdEncoding.EncodeToString(salt),
	}, nil
}

func (p KDFParams) validate() ([]byte, error) {
	if p.Algorithm != "argon2id" {
		return nil, fmt.Errorf("unsupported backup KDF %q", p.Algorithm)
	}
	if p.Time < 1 || p.Time > 10 {
		return nil, errors.New("backup KDF time cost outside allowed range")
	}
	if p.MemoryKiB < 8*1024 || p.MemoryKiB > 256*1024 {
		return nil, errors.New("backup KDF memory cost outside allowed range")
	}
	if p.Threads < 1 || p.Threads > 8 {
		return nil, errors.New("backup KDF parallelism outside allowed range")
	}
	if p.KeyLength != argonKeyLength {
		return nil, errors.New("backup KDF key length must be 32 bytes")
	}
	salt, err := base64.RawStdEncoding.DecodeString(p.Salt)
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return nil, errors.New("backup KDF salt is invalid")
	}
	return salt, nil
}

func deriveKey(password []byte, params KDFParams) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("backup password must not be empty")
	}
	salt, err := params.validate()
	if err != nil {
		return nil, err
	}
	return argon2.IDKey(password, salt, params.Time, params.MemoryKiB, params.Threads, params.KeyLength), nil
}

func encryptPayload(password, plaintext []byte) (Envelope, error) {
	params, err := newKDFParams()
	if err != nil {
		return Envelope{}, err
	}
	key, err := deriveKey(password, params)
	if err != nil {
		return Envelope{}, err
	}
	defer zero(key)

	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, fmt.Errorf("create backup cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, fmt.Errorf("create backup AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, fmt.Errorf("generate backup nonce: %w", err)
	}
	envelope := Envelope{Magic: FormatMagic, Version: SchemaVersion, KDF: params}
	aad, err := json.Marshal(envelopeAAD{Magic: envelope.Magic, Version: envelope.Version, KDF: envelope.KDF})
	if err != nil {
		return Envelope{}, fmt.Errorf("encode backup authenticated header: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	envelope.Nonce = base64.RawStdEncoding.EncodeToString(nonce)
	envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
	return envelope, nil
}

func decryptPayload(password []byte, envelope Envelope) ([]byte, error) {
	if envelope.Magic != FormatMagic || envelope.Version != SchemaVersion {
		return nil, errors.New("unsupported application backup format")
	}
	key, err := deriveKey(password, envelope.KDF)
	if err != nil {
		return nil, err
	}
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create backup cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create backup AEAD: %w", err)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errors.New("backup nonce is invalid")
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("backup ciphertext encoding is invalid")
	}
	if int64(len(ciphertext)) > MaxPayloadBytes+64<<20 {
		return nil, errors.New("backup ciphertext exceeds maximum size")
	}
	aad, err := json.Marshal(envelopeAAD{Magic: envelope.Magic, Version: envelope.Version, KDF: envelope.KDF})
	if err != nil {
		return nil, fmt.Errorf("encode backup authenticated header: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("backup authentication failed")
	}
	return plaintext, nil
}

func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
