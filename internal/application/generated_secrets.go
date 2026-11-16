package application

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

const generatedSecretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func GenerateSecretValue(generation SecretGeneration) ([]byte, error) {
	switch generation.Type {
	case "random":
		if err := validateSecretGeneration("generated", &generation); err != nil {
			return nil, err
		}
		return generateRandomText(generation.Length)
	case "hex":
		if err := validateSecretGeneration("generated", &generation); err != nil {
			return nil, err
		}
		raw := make([]byte, generation.Bytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, errors.New("generate cryptographically secure random bytes failed")
		}
		encoded := make([]byte, hex.EncodedLen(len(raw)))
		hex.Encode(encoded, raw)
		clear(raw)
		return encoded, nil
	default:
		return nil, errors.New("unsupported generated secret type")
	}
}

func generateRandomText(length int) ([]byte, error) {
	result := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return nil, errors.New("generate cryptographically secure random bytes failed")
	}
	for i, value := range random {
		result[i] = generatedSecretAlphabet[int(value)%len(generatedSecretAlphabet)]
	}
	clear(random)
	return result, nil
}
