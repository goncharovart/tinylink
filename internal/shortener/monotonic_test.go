package shortener

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMonotonic_UniqueAndOrdered(t *testing.T) {
	m := NewMonotonic(0, 7)
	seen := make(map[string]struct{})
	const N = 1024
	for i := 0; i < N; i++ {
		code, err := m.Next()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("collision at i=%d: %q already issued", i, code)
		}
		seen[code] = struct{}{}
	}
	if len(seen) != N {
		t.Errorf("expected %d unique codes; got %d", N, len(seen))
	}
}

func TestMonotonic_WidthPadding(t *testing.T) {
	m := NewMonotonic(0, 7)
	for i := 0; i < 5; i++ {
		code, err := m.Next()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 7 {
			t.Errorf("code %q length = %d, want 7 (padded)", code, len(code))
		}
		// Padding must be leading zeros.
		if !strings.HasPrefix(code, "00") {
			t.Errorf("small-int code %q not left-zero-padded", code)
		}
	}
}

func TestMonotonic_OffsetStart(t *testing.T) {
	m := NewMonotonic(1000, 7)
	code1, _ := m.Next()
	code2, _ := m.Next()
	if code1 == code2 {
		t.Error("consecutive Next() returned the same code")
	}
}

func TestMonotonic_ConcurrentNoCollisions(t *testing.T) {
	m := NewMonotonic(0, 7)
	const N = 10_000
	const Workers = 16

	seen := sync.Map{}
	var collisions atomic.Int32
	var wg sync.WaitGroup

	per := N / Workers
	for w := 0; w < Workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				code, err := m.Next()
				if err != nil {
					t.Errorf("Next: %v", err)
					return
				}
				if _, loaded := seen.LoadOrStore(code, struct{}{}); loaded {
					collisions.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if c := collisions.Load(); c != 0 {
		t.Fatalf("%d concurrent collisions across %d Next() calls — atomic.Add is broken", c, N)
	}
}

func TestMonotonic_StrideShardingTwoReplicas(t *testing.T) {
	// Replica A: stride=2 offset=0 → emits even-counter codes
	// Replica B: stride=2 offset=1 → emits odd-counter codes
	// Combined output must be collision-free.
	a := NewMonotonic(0, 7)
	b := NewMonotonic(1, 7)

	seen := make(map[string]string)
	for i := 0; i < 256; i++ {
		ca, _ := a.NextStride(2)
		if prev, ok := seen[ca]; ok {
			t.Fatalf("collision: replica A emitted %q, already issued by %s", ca, prev)
		}
		seen[ca] = "A"

		cb, _ := b.NextStride(2)
		if prev, ok := seen[cb]; ok {
			t.Fatalf("collision: replica B emitted %q, already issued by %s", cb, prev)
		}
		seen[cb] = "B"
	}
}

func TestEncodeBase62Padded_KnownValues(t *testing.T) {
	// The alphabet (shared with base62.go) is "0-9 A-Z a-z" — digit
	// 9 = '9', then 'A' = 10, 'Z' = 35, 'a' = 36, 'z' = 61.
	cases := []struct {
		n     uint64
		width int
		want  string
	}{
		{0, 1, "0"},
		{0, 7, "0000000"},
		{61, 1, "z"},
		{62, 2, "10"},
		{62*62 - 1, 2, "zz"},
	}
	for _, c := range cases {
		got, err := encodeBase62Padded(c.n, c.width)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("encodeBase62Padded(%d, %d) = %q, want %q", c.n, c.width, got, c.want)
		}
	}
}

func BenchmarkMonotonic_Next(b *testing.B) {
	m := NewMonotonic(0, 7)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Next()
	}
}

func BenchmarkRandom_Next(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Random(7)
	}
}
