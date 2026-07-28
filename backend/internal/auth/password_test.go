package auth

import "testing"

func TestPasswordHasher(t *testing.T) {
	hasher := NewPasswordHasher()
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("Hash() returned the plaintext password")
	}
	if !hasher.Compare(hash, "correct horse battery staple") {
		t.Fatal("Compare() = false, want true")
	}
	if hasher.Compare(hash, "wrong") {
		t.Fatal("Compare() for wrong password = true, want false")
	}
}
