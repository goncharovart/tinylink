// Package shortener encodes monotonically increasing integers as short
// base62 strings suitable for use as URL slugs.
//
// Base62 uses [0-9A-Za-z]; collisions are impossible by construction
// because each input integer has a unique encoding. Generators that
// guarantee uniqueness of the integer input (e.g. a Postgres sequence,
// a snowflake-style ID, or the in-process Monotonic allocator from
// stage 5) compose with this package without further collision handling.
package shortener

import (
	"crypto/rand"
	"errors"
	"math/big"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Encode converts a non-negative integer to its base62 representation.
// The empty string is returned for n == 0 — callers should ensure the
// generator never emits zero if they need a non-empty slug.
func Encode(n uint64) string {
	if n == 0 {
		return string(alphabet[0])
	}
	// Worst case: math.MaxUint64 → 11 base62 chars (62^11 > MaxUint64).
	buf := make([]byte, 0, 11)
	for n > 0 {
		buf = append(buf, alphabet[n%62])
		n /= 62
	}
	// Reverse in place.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// ErrInvalidCode indicates a Decode input contains a character outside
// the base62 alphabet.
var ErrInvalidCode = errors.New("shortener: code contains a non-base62 character")

// Decode reverses Encode. It returns ErrInvalidCode for malformed input.
func Decode(s string) (uint64, error) {
	var n uint64
	for i := 0; i < len(s); i++ {
		idx := indexOf(s[i])
		if idx == -1 {
			return 0, ErrInvalidCode
		}
		n = n*62 + uint64(idx)
	}
	return n, nil
}

func indexOf(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 36
	default:
		return -1
	}
}

// Random returns a base62 code of the given length using crypto/rand.
// It is the fallback when the storage layer does not provide a
// monotonic ID source (e.g. for the naive baseline before a real
// sequence-backed scheme is wired up).
//
// Collision probability for length=7: ~1 in 62^7 ≈ 1 in 3.5×10¹². At
// 100M URLs the birthday-paradox collision probability is still
// negligible (~1 in 70k); the storage layer's UNIQUE constraint on
// `code` is the authoritative guard.
func Random(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("shortener: length must be > 0")
	}
	buf := make([]byte, length)
	max := big.NewInt(62)
	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[idx.Int64()]
	}
	return string(buf), nil
}
