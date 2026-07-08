package fixtures

import (
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// FlagService's client field is never assigned or composite-literal-bound
// anywhere in this scanned tree — the dominant Go dependency-injection
// pattern (found via flaglint/corpus's struct-field-receiver fixture):
// the client is wired up by an external framework or a constructor not
// included in what's actually scanned. The field's *declared type* alone
// (`*ld.LDClient`) is the only proof available, and it's sufficient: Go's
// own type system guarantees the field can only ever hold an SDK client,
// the same soundness paramClientBindings already relies on for function
// parameters (factory.go) — just never extended to struct fields before.
type FlagService struct {
	ld *ld.LDClient
}

func (s *FlagService) DarkMode(userID string) bool {
	v, _ := s.ld.BoolVariation("declared-type-only-flag", nil, false)
	return v
}

// unrelatedFieldOwner's field is a plain string, not an SDK client — the
// field's own name ("ld") coincidentally overlaps with a common alias,
// but its declared type never resolves via starLDClientType, proving
// this mechanism keys strictly off the resolved type, never the name.
type unrelatedFieldOwner struct {
	ld string
}

func (u *unrelatedFieldOwner) Report() string {
	return u.ld
}
