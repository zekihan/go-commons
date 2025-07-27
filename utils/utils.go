package utils

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var ErrNegativeLength = errors.New("length cannot be negative")

func RandString(nByte int) (string, error) {
	if nByte < 0 {
		return "", ErrNegativeLength
	}

	randomBytes := make([]byte, nByte)
	if nByte == 0 {
		return "", nil // Return empty string for zero length
	}

	_, err := io.ReadFull(rand.Reader, randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
