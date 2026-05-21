package shortener

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestEncodeDecodeRoundtrip(t *testing.T) {
	cases := []uint64{0, 1, 61, 62, 63, 1000, 1_000_000, math.MaxUint32, math.MaxUint64 - 1}
	for _, n := range cases {
		encoded := Encode(n)
		if encoded == "" {
			t.Errorf("Encode(%d) returned empty string", n)
			continue
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode(%q) failed for input %d: %v", encoded, n, err)
			continue
		}
		if decoded != n {
			t.Errorf("roundtrip: Encode(%d) = %q, Decode(...) = %d", n, encoded, decoded)
		}
	}
}

func TestEncode_Length(t *testing.T) {
	// 62^7 > 3.5×10¹² — a 7-char encoding covers a large URL space.
	got := Encode(1_000_000_000)
	if len(got) > 7 {
		t.Errorf("Encode(1B) = %q (len %d), want ≤7", got, len(got))
	}
}

func TestDecode_RejectsInvalidChar(t *testing.T) {
	_, err := Decode("abc!")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode for non-base62 input, got %v", err)
	}
}

func TestRandom(t *testing.T) {
	t.Run("rejects non-positive length", func(t *testing.T) {
		if _, err := Random(0); err == nil {
			t.Fatal("expected error for length=0")
		}
		if _, err := Random(-1); err == nil {
			t.Fatal("expected error for length=-1")
		}
	})

	t.Run("returns code of requested length", func(t *testing.T) {
		got, err := Random(7)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 7 {
			t.Errorf("len(Random(7)) = %d, want 7", len(got))
		}
	})

	t.Run("uses only base62 alphabet", func(t *testing.T) {
		got, _ := Random(50)
		for i := 0; i < len(got); i++ {
			if !strings.ContainsRune(alphabet, rune(got[i])) {
				t.Errorf("Random produced non-alphabet byte: %q", got[i])
			}
		}
	})
}
