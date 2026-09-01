package sqlite

import "testing"

func TestBuildFTSQuery(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"  ":         "",
		"photo":      "photo*",
		"my photo":   "my AND photo*",
		`foo"bar*`:   "foobar*",
	}
	for in, want := range cases {
		if got := BuildFTSQuery(in); got != want {
			t.Errorf("BuildFTSQuery(%q)=%q want %q", in, got, want)
		}
	}
}
