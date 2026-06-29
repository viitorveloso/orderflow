// Package auth provides password hashing and JWT issuing/verification.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Password hashing parameters.
//
// We use PBKDF2-HMAC-SHA256 (RFC 8018). The iteration count follows the OWASP
// Password Storage Cheat Sheet recommendation for PBKDF2-HMAC-SHA256. PBKDF2 is
// FIPS-140 approved, which is why it is preferred here over bcrypt/argon2id;
// for a deployment without FIPS constraints, argon2id is an equally reasonable
// choice. The cryptographic core (HMAC-SHA256) is the standard library's vetted
// implementation — only the standard PBKDF2 iteration construction is assembled
// here, and it is verified against a known test vector in password_test.go.
const (
	pbkdf2Iterations = 600_000
	pbkdf2KeyLen     = 32 // bytes (256-bit derived key)
	saltLen          = 16 // bytes (128-bit salt)
	hashAlgorithm    = "pbkdf2-sha256"
)

// ErrInvalidHash is returned when a stored hash cannot be parsed.
var ErrInvalidHash = errors.New("invalid password hash format")

// HashPassword derives an encoded hash from a plaintext password using a fresh
// random salt. The returned string is self-describing:
//
//	pbkdf2-sha256$<iterations>$<base64-salt>$<base64-derived-key>
//
// Encoding the algorithm and parameters alongside the hash means the cost can be
// raised later without invalidating existing hashes — old hashes still verify
// with their original parameters while new hashes use the new ones.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	dk := pbkdf2SHA256([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen)

	return fmt.Sprintf(
		"%s$%d$%s$%s",
		hashAlgorithm,
		pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk),
	), nil
}

// VerifyPassword reports whether password matches the encoded hash. The
// comparison is constant time to avoid leaking information through timing.
func VerifyPassword(password, encoded string) (bool, error) {
	algo, iter, salt, want, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	if algo != hashAlgorithm {
		return false, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidHash, algo)
	}

	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// parseHash splits an encoded hash into its components.
func parseHash(encoded string) (algo string, iter int, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return "", 0, nil, nil, ErrInvalidHash
	}

	iter, err = strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return "", 0, nil, nil, fmt.Errorf("%w: bad iteration count", ErrInvalidHash)
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[2]); err != nil {
		return "", 0, nil, nil, fmt.Errorf("%w: bad salt", ErrInvalidHash)
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[3]); err != nil {
		return "", 0, nil, nil, fmt.Errorf("%w: bad key", ErrInvalidHash)
	}
	return parts[0], iter, salt, key, nil
}

// pbkdf2SHA256 implements PBKDF2 (RFC 8018, section 5.2) with HMAC-SHA256 as the
// pseudorandom function. It is equivalent to Python's
// hashlib.pbkdf2_hmac("sha256", ...) and golang.org/x/crypto/pbkdf2.Key with
// sha256.New.
func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	derived := make([]byte, 0, numBlocks*hashLen)
	block := make([]byte, 4)
	u := make([]byte, hashLen)
	t := make([]byte, hashLen)

	for blockNum := 1; blockNum <= numBlocks; blockNum++ {
		// U_1 = PRF(password, salt || INT_32_BE(blockNum))
		binary.BigEndian.PutUint32(block, uint32(blockNum))
		prf.Reset()
		prf.Write(salt)
		prf.Write(block)
		u = prf.Sum(u[:0])
		copy(t, u)

		// U_c = PRF(password, U_{c-1}); T_block = U_1 XOR U_2 XOR ... XOR U_iterations
		for c := 2; c <= iterations; c++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range t {
				t[i] ^= u[i]
			}
		}
		derived = append(derived, t...)
	}

	return derived[:keyLen]
}
