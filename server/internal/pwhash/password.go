// Package pwhash implements argon2id password hashing for page passwords,
// using the PHC string format so the hashing parameters travel with the hash.
//
// Plaintext passwords must exist only transiently (inbound requests); nothing
// in this package logs or retains them.
package pwhash

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parameters for argon2id. Chosen per the project brief: time=1, memory=64MB,
// threads=4. They are encoded into every hash so they can be tuned later
// without invalidating existing hashes.
const (
	defaultTime    = 1
	defaultMemory  = 64 * 1024 // KiB => 64 MB
	defaultThreads = 4
	keyLen         = 32
	saltLen        = 16
)

// ErrInvalidHash is returned by Verify when the stored hash is malformed.
var ErrInvalidHash = errors.New("pwhash: malformed argon2id hash")

// Hash returns the PHC-format argon2id encoded hash of password.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("pwhash: reading salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, defaultTime, defaultMemory, defaultThreads, keyLen)
	return encode(defaultTime, defaultMemory, defaultThreads, salt, key), nil
}

// Verify reports whether password matches the encoded argon2id hash.
// The comparison is constant-time. A malformed hash yields (false, ErrInvalidHash).
func Verify(password, encoded string) (bool, error) {
	time, memory, threads, salt, wantKey, err := decode(encoded)
	if err != nil {
		return false, err
	}
	gotKey := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(wantKey)))
	return hmac.Equal(gotKey, wantKey), nil
}

func encode(time, memory uint32, threads uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, time, threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decode(encoded string) (time, memory uint32, threads uint8, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<key>"
	if len(parts) != 6 || parts[1] != "argon2id" {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	for _, param := range strings.Split(parts[3], ",") {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) != 2 {
			return 0, 0, 0, nil, nil, ErrInvalidHash
		}
		v, perr := strconv.ParseUint(kv[1], 10, 32)
		if perr != nil {
			return 0, 0, 0, nil, nil, ErrInvalidHash
		}
		switch kv[0] {
		case "m":
			memory = uint32(v)
		case "t":
			time = uint32(v)
		case "p":
			threads = uint8(v)
		default:
			return 0, 0, 0, nil, nil, ErrInvalidHash
		}
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	if len(salt) == 0 || len(key) == 0 || time == 0 || memory == 0 || threads == 0 {
		return 0, 0, 0, nil, nil, ErrInvalidHash
	}
	return time, memory, threads, salt, key, nil
}
