package kit

import "testing"

func TestShortCount(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{3900, "3.9k"},
		{999999, "1000.0k"},
		{1000000, "1.0m"},
		{2400000, "2.4m"},
	}
	for _, c := range cases {
		if got := shortCount(c.in); got != c.want {
			t.Errorf("shortCount(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
