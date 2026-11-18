package desktop

import "testing"

func TestDiagMenuLabel(t *testing.T) {
	if got := diagMenuLabel(0); got != "启动诊断…" {
		t.Errorf("n=0 label = %q", got)
	}
	if got := diagMenuLabel(-3); got != "启动诊断…" {
		t.Errorf("negative label = %q", got)
	}
	if got := diagMenuLabel(2); got != "启动诊断（2 个问题）…" {
		t.Errorf("n=2 label = %q", got)
	}
}

// compile-time check: a Controller implementation must gain ShowDiagnostics.
type diagControllerProbe struct{}

func (diagControllerProbe) ToggleServe()       {}
func (diagControllerProbe) Rebuild()           {}
func (diagControllerProbe) CopyAdminToken()    {}
func (diagControllerProbe) OpenSiteFolder()    {}
func (diagControllerProbe) OpenSiteInBrowser() {}
func (diagControllerProbe) SwitchSite(string)  {}
func (diagControllerProbe) ChooseSite()        {}
func (diagControllerProbe) OpenLogs()          {}
func (diagControllerProbe) ShowWindow()        {}
func (diagControllerProbe) Quit()              {}
func (diagControllerProbe) ShowDiagnostics()   {}

var _ Controller = diagControllerProbe{}
