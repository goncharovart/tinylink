package shortener

import (
	"encoding/binary"
	"errors"
	"strings"
	"sync/atomic"
)

// Monotonic is a collision-free code allocator. It hands out codes
// derived from a process-local monotonic counter, base62-encoded.
//
// vs Random + retry (the stage-0 allocator):
//
//   - Random allocation has a known long-tail latency spike. As the
//     code-space fills up, the collision probability grows with the
//     birthday paradox; the retry loop hides it under p99 but the
//     mean grows too.
//   - Monotonic allocation produces a guaranteed-unique code in a
//     single CPU operation. No DB roundtrip, no retry on conflict.
//
// Trade-offs Monotonic does NOT solve:
//
//   - Predictability. The next code is *guessable* by anyone watching
//     two consecutive Save() calls. For tinylink that is acceptable
//     because the protected resource is the destination URL — the
//     guesser still has to do a redirect roundtrip to harvest the
//     mapping, and the redirect path is rate-limited (issue #3).
//   - Distributed coordination. A single process is the source of
//     truth for "next code". In a multi-replica deployment you need
//     to either shard the counter range per replica (Snowflake-style)
//     or back the counter with a Postgres SEQUENCE — both available
//     as follow-up issues.
//
// The Monotonic allocator is exposed as a stage-5 *opt-in*; the
// stage-0 random allocator remains the default so the four-stage
// walkthrough table at the top of the README stays comparable.
type Monotonic struct {
	counter atomic.Uint64
	width   int
}

// NewMonotonic returns a fresh allocator starting at offset.
//
// width controls the minimum length of the emitted code (zero-pad
// with '0' on the left). 7 matches the random allocator's default so
// the two are visually indistinguishable to the redirect handler.
//
// offset lets two replicas share one DB without colliding: replica A
// starts at 0 stride 2, replica B at 1 stride 2, by passing
// different offsets and using NextStride to skip. Not needed for the
// single-process case — pass 0.
func NewMonotonic(offset uint64, width int) *Monotonic {
	if width <= 0 {
		width = 7
	}
	m := &Monotonic{width: width}
	m.counter.Store(offset)
	return m
}

// Next returns the next unique code.
func (m *Monotonic) Next() (string, error) {
	n := m.counter.Add(1) - 1 // serve from offset, not offset+1
	return encodeBase62Padded(n, m.width)
}

// NextStride returns the code at the current counter then advances
// the counter by stride. Useful for sharded multi-replica setups
// (replica A: stride=2 offset=0; replica B: stride=2 offset=1).
//
// stride must be ≥ 1; stride=1 is equivalent to Next.
func (m *Monotonic) NextStride(stride uint64) (string, error) {
	if stride < 1 {
		return "", errors.New("shortener: stride must be ≥ 1")
	}
	// First read the current value, then atomically bump by stride.
	for {
		cur := m.counter.Load()
		if m.counter.CompareAndSwap(cur, cur+stride) {
			return encodeBase62Padded(cur, m.width)
		}
	}
}

// encodeBase62Padded renders n in base62 left-padded with '0' to at
// least width characters. Uses the same `alphabet` constant defined
// in base62.go (digits first → A-Z → a-z).
func encodeBase62Padded(n uint64, width int) (string, error) {
	if width < 1 {
		return "", errors.New("shortener: width must be ≥ 1")
	}
	if n == 0 {
		return strings.Repeat("0", width), nil
	}
	var buf [16]byte // 16 base62 chars handles up to 62^16 ≈ 4.7×10^28, more than uint64
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = alphabet[n%62]
		n /= 62
	}
	out := string(buf[i:])
	if len(out) >= width {
		return out, nil
	}
	return strings.Repeat("0", width-len(out)) + out, nil
}

// encodeBase62Uint8 is a small helper used by tests to round-trip an
// encoded code back through binary.BigEndian for sanity checking. It
// is NOT a decoder for the wire path — the redirect handler does not
// decode codes; it looks them up in storage by the string itself.
func encodeBase62Uint8(n uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], n)
	return b[:]
}
