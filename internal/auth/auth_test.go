package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTokenRoundTrip(t *testing.T) {
	tm := NewTokenManager("super-secret", time.Hour)
	id := uuid.New()

	token, expiresAt, err := tm.Generate(id, "alex.carter@hybreed.app")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiry should be in the future, got %v", expiresAt)
	}

	gotID, gotEmail, err := tm.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gotID != id {
		t.Errorf("id mismatch: got %s want %s", gotID, id)
	}
	if gotEmail != "alex.carter@hybreed.app" {
		t.Errorf("email mismatch: got %s", gotEmail)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	signer := NewTokenManager("secret-a", time.Hour)
	verifier := NewTokenManager("secret-b", time.Hour)

	token, _, err := signer.Generate(uuid.New(), "x@y.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, _, err := verifier.Parse(token); err == nil {
		t.Fatal("expected parse to fail with a different secret")
	}
}

func TestParseRejectsExpired(t *testing.T) {
	tm := NewTokenManager("secret", time.Hour)
	tm.now = func() time.Time { return time.Now().Add(-2 * time.Hour) } // token already expired

	token, _, err := tm.Generate(uuid.New(), "x@y.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, _, err := tm.Parse(token); err == nil {
		t.Fatal("expected parse to reject an expired token")
	}
}

func TestPasswordHashAndCheck(t *testing.T) {
	hash, err := HashPassword("trainhard")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "trainhard" {
		t.Fatal("password must not be stored in plaintext")
	}
	if !CheckPassword(hash, "trainhard") {
		t.Error("correct password should verify")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password should not verify")
	}
}

func TestGenerateOTPIsSixDigits(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := generateOTP()
		if err != nil {
			t.Fatalf("generate otp: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("expected 6 digits, got %q", code)
		}
		for _, c := range code {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit in otp: %q", code)
			}
		}
	}
}

func TestSha256HexDeterministic(t *testing.T) {
	first := sha256hex("123456")
	second := sha256hex("123456")
	if first != second {
		t.Error("hash should be deterministic")
	}
	if sha256hex("123456") == sha256hex("654321") {
		t.Error("different inputs should hash differently")
	}
}
