package sshkeys

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateEphemeralKeyPair(t *testing.T) {
	keyPair, err := GenerateEphemeralKeyPair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeyPair() error = %v", err)
	}

	if keyPair == nil {
		t.Fatal("GenerateEphemeralKeyPair() returned nil")
	}

	// Verify signer is valid
	if keyPair.Signer() == nil {
		t.Error("Signer() returned nil")
	}

	// Verify public key is valid
	if keyPair.PublicKey() == nil {
		t.Error("PublicKey() returned nil")
	}

	// Verify public key string format
	pubKeyStr := keyPair.PublicKeyString()
	if !strings.HasPrefix(pubKeyStr, "ssh-ed25519 ") {
		t.Errorf("PublicKeyString() = %q, want prefix 'ssh-ed25519 '", pubKeyStr)
	}

	// Verify auth method is valid
	authMethod := keyPair.AuthMethod()
	if authMethod == nil {
		t.Error("AuthMethod() returned nil")
	}
}

func TestGenerateEphemeralKeyPair_UniqueKeys(t *testing.T) {
	keyPair1, err := GenerateEphemeralKeyPair()
	if err != nil {
		t.Fatalf("First GenerateEphemeralKeyPair() error = %v", err)
	}

	keyPair2, err := GenerateEphemeralKeyPair()
	if err != nil {
		t.Fatalf("Second GenerateEphemeralKeyPair() error = %v", err)
	}

	// Keys should be different
	if keyPair1.PublicKeyString() == keyPair2.PublicKeyString() {
		t.Error("Generated keys should be unique")
	}
}

func TestEphemeralKeyPair_SignAndVerify(t *testing.T) {
	keyPair, err := GenerateEphemeralKeyPair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeyPair() error = %v", err)
	}

	// Test signing
	testData := []byte("test data to sign")
	sig, err := keyPair.Signer().Sign(nil, testData)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify signature
	err = keyPair.Signer().PublicKey().Verify(testData, sig)
	if err != nil {
		t.Errorf("Verify() error = %v", err)
	}
}

func TestEphemeralKeyPair_PublicKeyParseable(t *testing.T) {
	keyPair, err := GenerateEphemeralKeyPair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeyPair() error = %v", err)
	}

	// The public key string should be parseable by ssh.ParseAuthorizedKey
	pubKeyStr := keyPair.PublicKeyString()
	_, _, _, _, err = ssh.ParseAuthorizedKey([]byte(pubKeyStr))
	if err != nil {
		t.Errorf("PublicKeyString() produced unparseable key: %v", err)
	}
}
