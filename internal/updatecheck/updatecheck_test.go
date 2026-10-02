package updatecheck

import (
	"testing"

	"github.com/blang/semver/v4"
)

func TestCompare(t *testing.T) {
	cur := semver.MustParse("1.2.3")
	cases := map[string]bool{"1.2.4": true, "v2.0.0": true, "1.2.3": false, "1.0.0": false, "garbage": false, "": false}
	for latest, want := range cases {
		if _, got := compare(latest, cur); got != want {
			t.Errorf("compare(%q) = %v, want %v", latest, got, want)
		}
	}
}
