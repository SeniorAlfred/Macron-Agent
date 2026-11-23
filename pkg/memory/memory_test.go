package memory

import "testing"

func TestEstimateTokensCountsCheaply(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 1},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"سلام", 2},
		{"سلام دنیا", 4},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.in); got != c.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateTokensNeverZeroForText(t *testing.T) {
	for _, s := range []string{"a", " ", "\n", "🙂"} {
		if got := EstimateTokens(s); got == 0 {
			t.Fatalf("EstimateTokens(%q) must not be zero", s)
		}
	}
}

func TestEstimateTokensGrowsWithTheText(t *testing.T) {
	small := EstimateTokens("one short line")
	big := EstimateTokens("one short line\nsecond line\nthird line\nfourth line\nfifth line")
	if big <= small {
		t.Fatalf("a longer text should cost more: %d <= %d", big, small)
	}
}
