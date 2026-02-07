package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateRSAKeyPair generates a new RSA key pair and saves it to the specified path
func GenerateRSAKeyPair(keyPath string, bitSize int) error {
	if bitSize == 0 {
		bitSize = 2048 // Default to 2048 bits
	}

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create private key file
	privateKeyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer privateKeyFile.Close()

	// Encode private key to PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	// Generate public key
	publicKey := &privateKey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create public key file
	publicKeyPath := keyPath + ".pub"
	publicKeyFile, err := os.OpenFile(publicKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer publicKeyFile.Close()

	// Encode public key to PEM format
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	if err := pem.Encode(publicKeyFile, publicKeyPEM); err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	return nil
}

// EnsureRSAKeyPair ensures that an RSA key pair exists at the specified path,
// generating a new one if it doesn't exist
func EnsureRSAKeyPair(keyPath string, bitSize int) error {
	// Check if private key exists
	if _, err := os.Stat(keyPath); err == nil {
		// Key exists, no need to generate
		return nil
	}

	// Key doesn't exist, generate it
	if err := GenerateRSAKeyPair(keyPath, bitSize); err != nil {
		return fmt.Errorf("failed to ensure RSA key pair: %w", err)
	}

	return nil
}

// RegenerateRSAKeyPair removes the old key and generates a new RSA key pair
func RegenerateRSAKeyPair(keyPath string, bitSize int) error {
	// Remove old private key if it exists
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old private key: %w", err)
	}

	// Remove old public key if it exists
	publicKeyPath := keyPath + ".pub"
	if err := os.Remove(publicKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old public key: %w", err)
	}

	// Generate new key pair
	if err := GenerateRSAKeyPair(keyPath, bitSize); err != nil {
		return fmt.Errorf("failed to regenerate RSA key pair: %w", err)
	}

	return nil
}

// KeyExists checks if an RSA key exists at the specified path
func KeyExists(keyPath string) bool {
	_, err := os.Stat(keyPath)
	return err == nil
}
