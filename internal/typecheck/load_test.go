package typecheck

import "testing"

func TestWithGotypesalias(t *testing.T) {
	cases := []struct {
		name        string
		current     string
		wantGodebug string
		wantChanged bool
	}{
		{"empty", "", "gotypesalias=1", true},
		{"unrelated setting present", "panicnil=1", "panicnil=1,gotypesalias=1", true},
		{"already set explicitly", "gotypesalias=0", "gotypesalias=0", false},
		{"already set alongside others", "panicnil=1,gotypesalias=0", "panicnil=1,gotypesalias=0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			godebug, changed := withGotypesalias(tc.current)
			if godebug != tc.wantGodebug || changed != tc.wantChanged {
				t.Errorf("withGotypesalias(%q) = (%q, %v), want (%q, %v)",
					tc.current, godebug, changed, tc.wantGodebug, tc.wantChanged)
			}
		})
	}
}
