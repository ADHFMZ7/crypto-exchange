package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	if hash == password {
		t.Fatal("password was stored in plaintext")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("hash %q is not in bcrypt's modular crypt format", hash)
	}
	if !CheckPasswordHash(password, hash) {
		t.Fatal("the password that produced the hash did not verify against it")
	}
}

func TestCheckPasswordHashRejects(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"wrong password":     "hunter3",
		"empty":              "",
		"case differs":       "Hunter2",
		"trailing space":     "hunter2 ",
		"prefix of correct":  "hunter",
		"correct plus extra": "hunter22",
	}

	for name, attempt := range cases {
		t.Run(name, func(t *testing.T) {
			if CheckPasswordHash(attempt, hash) {
				t.Fatalf("%q verified against a hash of %q", attempt, "hunter2")
			}
		})
	}
}

// bcrypt salts every hash, so the same password never produces the same string
// twice. Storing the salt inside the hash is what still lets both verify.
func TestHashesAreSalted(t *testing.T) {
	const password = "same-password"

	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("two hashes of the same password are identical: the salt is not random")
	}
	if !CheckPasswordHash(password, first) || !CheckPasswordHash(password, second) {
		t.Fatal("a salted hash failed to verify its own password")
	}
}

// A malformed or empty stored hash must fail closed. This is the shape of a
// database row that was never populated, and it must not authenticate anyone.
func TestCheckPasswordHashAgainstInvalidHash(t *testing.T) {
	for name, hash := range map[string]string{
		"empty":        "",
		"not bcrypt":   "plaintext-password",
		"truncated":    "$2a$10$abc",
		"wrong prefix": "$9z$10$" + strings.Repeat("a", 53),
	} {
		t.Run(name, func(t *testing.T) {
			if CheckPasswordHash("anything", hash) {
				t.Fatal("an invalid stored hash authenticated a login")
			}
			if CheckPasswordHash("", hash) {
				t.Fatal("an invalid stored hash authenticated an empty password")
			}
		})
	}
}

func TestEmptyPasswordStillHashes(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPasswordHash("", hash) {
		t.Fatal("empty password did not verify against its own hash")
	}
	if CheckPasswordHash("x", hash) {
		t.Fatal("non-empty password verified against a hash of the empty string")
	}
}

// bcrypt refuses passwords over 72 bytes rather than silently truncating them.
// Registration surfaces this as a 400, so it is worth pinning: the failure has
// to be an error, not a hash of the first 72 bytes.
func TestPasswordLongerThanBcryptLimitIsRejected(t *testing.T) {
	long := strings.Repeat("a", 73)

	if _, err := HashPassword(long); err == nil {
		t.Fatal("a 73-byte password was accepted; bcrypt's limit is 72")
	}

	// 72 is still fine.
	if _, err := HashPassword(strings.Repeat("a", 72)); err != nil {
		t.Fatalf("72-byte password should hash: %v", err)
	}
}
