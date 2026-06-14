package build

import (
	"os"
	"strings"
)

// strictI18nEnabled returns true when HUAN_STRICT_I18N env var is set to a
// truthy value ("true", "1", "yes"). Used by BuildSite to fail-fast on stale
// .en.md sidecars (CI mode) vs warn-only (local mode).
//
// Convention: CI workflows set HUAN_STRICT_I18N=true to block deploys that
// would publish translations out-of-sync with their source markdown. Local
// builds leave it unset so developers can iterate without re-translating
// on every change.
func strictI18nEnabled() bool {
	v := strings.ToLower(os.Getenv("HUAN_STRICT_I18N"))
	return v == "true" || v == "1" || v == "yes"
}
