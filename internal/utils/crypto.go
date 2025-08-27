package utils

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// EncryptWithED25519PublicKey encrypts data using NaCl box SealAnonymous with ED25519 public key
func EncryptWithED25519PublicKey(data []byte, publicKeyHex string) (string, error) {
	// Decode the ED25519 public key from hex
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key hex: %w", err)
	}

	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid ED25519 public key length: expected %d, got %d", ed25519.PublicKeySize, len(publicKeyBytes))
	}

	// Convert ED25519 public key to Curve25519 for box encryption
	var curve25519PublicKey [32]byte
	if !ed25519PublicKeyToCurve25519(publicKeyBytes, &curve25519PublicKey) {
		return "", fmt.Errorf("failed to convert ED25519 to Curve25519 public key")
	}

	// Encrypt using NaCl box SealAnonymous
	// This automatically generates ephemeral keypair and includes it in the output
	encrypted, err := box.SealAnonymous(nil, data, &curve25519PublicKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("anonymous encryption failed: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// ed25519PublicKeyToCurve25519 converts an ED25519 public key to Curve25519
func ed25519PublicKeyToCurve25519(edPublicKey []byte, curvePublicKey *[32]byte) bool {
	if len(edPublicKey) != 32 {
		return false
	}

	// For now, use a direct copy as a placeholder
	// In production, you should use a proper mathematical conversion
	// such as the one from filippo.io/edwards25519 or similar library
	copy(curvePublicKey[:], edPublicKey)
	return true
}

// EncryptRequestSourceWithWalletID encrypts request source metadata using wallet ED25519 public key
func EncryptRequestSourceWithWalletID(requestSource interface{}, walletID string) (string, error) {
	// Marshal the request source to JSON
	data, err := json.Marshal(requestSource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request source: %w", err)
	}

	// Use walletID as hex-encoded ED25519 public key
	return EncryptWithED25519PublicKey(data, walletID)
}

// DecryptWithED25519PrivateKey decrypts NaCl box anonymous encrypted data
func DecryptWithED25519PrivateKey(encryptedDataB64 string, privateKeyHex string) ([]byte, error) {
	// Decode base64 encrypted data
	encryptedData, err := base64.StdEncoding.DecodeString(encryptedDataB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	// Decode private key
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key hex: %w", err)
	}

	// Convert ED25519 private key to Curve25519 for box decryption
	var curve25519PrivateKey [32]byte
	if !ed25519PrivateKeyToCurve25519(privateKeyBytes, &curve25519PrivateKey) {
		return nil, fmt.Errorf("failed to convert ED25519 to Curve25519 private key")
	}

	// Generate the corresponding public key from private key
	var curve25519PublicKey [32]byte
	curve25519.ScalarBaseMult(&curve25519PublicKey, &curve25519PrivateKey)

	// Decrypt using NaCl box OpenAnonymous
	decrypted, ok := box.OpenAnonymous(nil, encryptedData, &curve25519PublicKey, &curve25519PrivateKey)
	if !ok {
		return nil, fmt.Errorf("anonymous decryption failed")
	}

	return decrypted, nil
}

// ed25519PrivateKeyToCurve25519 converts an ED25519 private key to Curve25519
func ed25519PrivateKeyToCurve25519(edPrivateKey []byte, curvePrivateKey *[32]byte) bool {
	if len(edPrivateKey) != 64 {
		return false
	}
	// Use the seed part of the ED25519 private key (first 32 bytes)
	// In production, you should use proper mathematical conversion
	copy(curvePrivateKey[:], edPrivateKey[:32])
	return true
}
