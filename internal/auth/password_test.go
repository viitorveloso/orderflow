package auth

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestPBKDF2KnownVector verifies the PBKDF2 implementation against a vector
// produced independently by Python's hashlib.pbkdf2_hmac("sha256", ...). This
// guards against any subtle bug in the iteration/XOR construction.
func TestPBKDF2KnownVector(t *testing.T) {
	// salt = bytes 0x00..0x0f, iterations = 600000, dklen = 32
	password := []byte("correct horse battery staple")
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(i)
	}
	const wantHex = "ef177144eec9420cbc1093d2a8b344a92bc506d0d4ec9c028dd19f8324d8c1e6"

	got := pbkdf2SHA256(password, salt, 600_000, 32)
	if gotHex := hex.EncodeToString(got); gotHex != wantHex {
		t.Fatalf("PBKDF2 mismatch:\n got=%s\nwant=%s", gotHex, wantHex)
	}
}

func TestHashAndVerifyRoundTrip(t *testing.T) {
	const password = "s3cret-passw0rd!"

	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !strings.HasPrefix(encoded, hashAlgorithm+"$") {
		t.Errorf("encoded hash missing algorithm prefix: %q", encoded)
	}

	ok, err := VerifyPassword(password, encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("correct password failed to verify")
	}

	ok, err = VerifyPassword("wrong-password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong): %v", err)
	}
	if ok {
		t.Error("wrong password verified as correct")
	}
}

func TestHashUsesUniqueSalt(t *testing.T) {
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"pbkdf2-sha256$notanumber$c2FsdA$aGFzaA",
		"pbkdf2-sha256$1000$@@@$aGFzaA",
		"unknown-algo$1000$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("expected error for malformed hash %q, got nil", c)
		}
	}
}
