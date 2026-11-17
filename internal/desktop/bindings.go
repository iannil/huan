package desktop

import (
	"fmt"

	"github.com/egoist/mygo"

	"github.com/iannil/huan/internal/version"
)

// DesktopBindings is the minimal typed IPC surface exposed to the admin SPA.
// Phase 1 exists to prove the TrustedOrigins + bindings path; the generated
// client is kept for future use and the SPA is not modified.
type DesktopBindings struct {
	app *App
}

// AppVersion returns the huan version string.
func (b DesktopBindings) AppVersion() string { return version.String() }

// RevealSiteFolder reveals the current site root in Finder.
func (b DesktopBindings) RevealSiteFolder() error {
	dir := b.app.currentSourceDir()
	if dir == "" {
		return fmt.Errorf("no site selected")
	}
	mygo.Shell.ShowItemInFolder(dir)
	return nil
}
