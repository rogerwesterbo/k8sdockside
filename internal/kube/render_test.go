package kube

import "testing"

func TestAgeFormatsLikeKubectl(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{0, "0s"},
		{45, "45m"},
		{90, "1h30m"},
		{60 * 12, "12h"},
		{60 * 30, "30h"},
		{60 * 24 * 3, "3d"},
		{60*24*3 + 60*5, "3d5h"},
		{60 * 24 * 400, "400d"},
	}
	for _, c := range cases {
		if got := age(c.minutes); got != c.want {
			t.Errorf("age(%d) = %q, want %q", c.minutes, got, c.want)
		}
	}
}

func TestToneForColoursTheStatesThatMatter(t *testing.T) {
	cases := map[string]string{
		"Running":          "ok",
		"Bound":            "ok",
		"Pending":          "warn",
		"CrashLoopBackOff": "error",
		"OOMKilled":        "error",
		"Whatever":         "",
	}
	for status, want := range cases {
		if got := toneFor(status); got != want {
			t.Errorf("toneFor(%q) = %q, want %q", status, got, want)
		}
	}
}
