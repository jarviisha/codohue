package compute

import "testing"

func TestMax32(t *testing.T) {
	cases := []struct {
		a, b, want float32
	}{
		{1, 2, 2},
		{2, 1, 2},
		{-1, -2, -1},
		{3, 3, 3},
		{0, 0, 0},
	}
	for _, c := range cases {
		if got := max32(c.a, c.b); got != c.want {
			t.Errorf("max32(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
