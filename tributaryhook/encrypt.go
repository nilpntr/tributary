package tributaryhook

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/nilpntr/tributary/tributarytype"
	"golang.org/x/crypto/nacl/secretbox"
)

// Encryptor provides an interface for encrypting and decrypting step arguments.
type Encryptor interface {
	// Encrypt encrypts the given plaintext data.
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts the given ciphertext data.
	Decrypt(ciphertext []byte) ([]byte, error)
}

// EncryptHook is a hook that encrypts step arguments before insertion
// and decrypts them before execution.
type EncryptHook struct {
	BaseHook
	encryptor Encryptor
}

// NewEncryptHook creates a new encryption hook with the given encryptor.
func NewEncryptHook(encryptor Encryptor) *EncryptHook {
	return &EncryptHook{
		encryptor: encryptor,
	}
}

// BeforeInsert encrypts the step arguments before insertion.
func (h *EncryptHook) BeforeInsert(ctx context.Context, args tributarytype.StepArgs, argsBytes []byte) ([]byte, error) {
	encrypted, err := h.encryptor.Encrypt(argsBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt args: %w", err)
	}
	return encrypted, nil
}

// BeforeWork decrypts the step arguments before execution.
func (h *EncryptHook) BeforeWork(ctx context.Context, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	decrypted, err := h.encryptor.Decrypt(argsBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt args: %w", err)
	}
	return decrypted, nil
}

// AfterWork encrypts the step result after execution.
func (h *EncryptHook) AfterWork(ctx context.Context, stepID int64, result []byte, err error) ([]byte, error) {
	if err != nil || result == nil {
		return result, nil
	}

	encrypted, encErr := h.encryptor.Encrypt(result)
	if encErr != nil {
		return nil, fmt.Errorf("failed to encrypt result: %w", encErr)
	}
	return encrypted, nil
}

// SecretboxEncryptor implements the Encryptor interface using NaCl secretbox.
// It provides authenticated encryption with a symmetric key.
type SecretboxEncryptor struct {
	key [32]byte
}

// NewSecretboxEncryptor creates a new Secretbox encryptor with the given 32-byte key.
func NewSecretboxEncryptor(key [32]byte) *SecretboxEncryptor {
	return &SecretboxEncryptor{
		key: key,
	}
}

// Encrypt encrypts the plaintext using NaCl secretbox.
// The nonce is randomly generated and prepended to the ciphertext.
func (e *SecretboxEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate a random nonce
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	// The nonce is prepended to the ciphertext
	encrypted := secretbox.Seal(nonce[:], plaintext, &nonce, &e.key)
	return encrypted, nil
}

// Decrypt decrypts the ciphertext using NaCl secretbox.
// The nonce is expected to be prepended to the ciphertext.
func (e *SecretboxEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 24 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract the nonce from the beginning of the ciphertext
	var nonce [24]byte
	copy(nonce[:], ciphertext[:24])

	// Decrypt the ciphertext
	decrypted, ok := secretbox.Open(nil, ciphertext[24:], &nonce, &e.key)
	if !ok {
		return nil, fmt.Errorf("decryption failed")
	}

	return decrypted, nil
}
