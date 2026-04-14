package ocp

import (
	"testing"
	"time"
)

func TestCallbackTokenSigner_RoundTrip(t *testing.T) {
	signer, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner: %v", err)
	}

	const clusterID = "test-cluster-123"
	token, err := signer.Sign(clusterID, 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	got, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != clusterID {
		t.Fatalf("subject mismatch: got %q, want %q", got, clusterID)
	}
}

func TestCallbackTokenSigner_ExpiredToken(t *testing.T) {
	signer, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner: %v", err)
	}

	token, err := signer.Sign("expired-cluster", -1*time.Second)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = signer.Verify(token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestCallbackTokenSigner_WrongAudience(t *testing.T) {
	signer, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner: %v", err)
	}

	token := signer.signWithAudience(t, "aud-cluster", "wrong-audience", 5*time.Minute)

	_, err = signer.Verify(token)
	if err == nil {
		t.Fatal("expected error for wrong audience, got nil")
	}
}

func TestCallbackTokenSigner_DifferentSignerRejects(t *testing.T) {
	signer1, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner signer1: %v", err)
	}
	signer2, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner signer2: %v", err)
	}

	token, err := signer1.Sign("cross-cluster", 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = signer2.Verify(token)
	if err == nil {
		t.Fatal("expected error when verifying with different signer, got nil")
	}
}

func TestCallbackTokenSigner_TamperedToken(t *testing.T) {
	signer, err := NewCallbackTokenSigner()
	if err != nil {
		t.Fatalf("NewCallbackTokenSigner: %v", err)
	}

	token, err := signer.Sign("tamper-cluster", 5*time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Flip the last byte of the token to tamper with it.
	tampered := token[:len(token)-1]
	lastByte := token[len(token)-1]
	if lastByte == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}

	_, err = signer.Verify(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token, got nil")
	}
}
