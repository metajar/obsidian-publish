package pwhash

import (
	"strings"
	"testing"
)

func TestHashVerifyRoundtrip(t *testing.T) {
	h, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=1,p=4$") {
		t.Errorf("hash missing expected params prefix, got %q", h)
	}
	ok, err := Verify("correct horse battery staple", h)
	if err != nil || !ok {
		t.Errorf("Verify correct password = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	h, err := Hash("s3cret")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	ok, err := Verify("wrong", h)
	if err != nil {
		t.Fatalf("Verify wrong password returned err: %v", err)
	}
	if ok {
		t.Error("Verify wrong password = true, want false")
	}
}

func TestHashUniqueSalts(t *testing.T) {
	a, _ := Hash("same password")
	b, _ := Hash("same password")
	if a == b {
		t.Error("two hashes of the same password are identical (salt not random?)")
	}
}

func TestVerifyMalformedHash(t *testing.T) {
	bad := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=1,p=4",
		"$argon2id$v=19$m=65536,t=1,p=4$onlysalt",
		"$argon2id$v=19$m=65536,t=0,p=4$ c2FsdA $a2V5", // t=0 rejected
		"$argon2i$v=19$m=65536,t=1,p=4$c2FsdA$a2V5",    // wrong variant
		"$argon2id$v=99$m=65536,t=1,p=4$c2FsdA$a2V5",   // unknown version
		"$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$!!!notb64",
	}
	for _, in := range bad {
		if ok, err := Verify("x", in); ok || err == nil {
			t.Errorf("Verify(%q) = (%v, %v), want (false, error)", in, ok, err)
		}
	}
}

func TestVerifyTamperedKey(t *testing.T) {
	h, err := Hash("password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	// Flip the last char of the key component to a different valid base64
	// char (a mirror flip could land outside the alphabet and turn this into
	// a parse-error test instead).
	repl := byte('A')
	if h[len(h)-1] == 'A' {
		repl = 'B'
	}
	tampered := h[:len(h)-1] + string(repl)
	ok, err := Verify("password", tampered)
	if err != nil {
		t.Fatalf("Verify tampered returned err: %v", err)
	}
	if ok {
		t.Error("Verify tampered hash = true, want false")
	}
}
