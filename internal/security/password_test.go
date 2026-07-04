package security

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword("correct-horse-battery-staple", hash) {
		t.Fatal("VerifyPassword() = false for correct password")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("VerifyPassword() = true for wrong password")
	}
}

func TestSessionTokenHashStable(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken() error = %v", err)
	}
	if hash == "" {
		t.Fatal("session hash is empty")
	}
	if SessionTokenHash(token) != hash {
		t.Fatal("session hash is not stable")
	}
}
