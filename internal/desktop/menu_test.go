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
type stubController struct{}

func (stubController) ToggleServe()       {}
func (stubController) Rebuild()           {}
func (stubController) CopyAdminToken()    {}
func (stubController) OpenSiteFolder()    {}
func (stubController) OpenSiteInBrowser() {}
func (stubController) SwitchSite(string)  {}
func (stubController) ChooseSite()        {}
func (stubController) OpenLogs()          {}
func (stubController) ShowWindow()        {}
func (stubController) Quit()              {}
func (stubController) ShowDiagnostics()   {}

var _ Controller = stubController{}

func TestBuildTrayMenu_OmitsTokenItem_WhenAuthDisabled(t *testing.T) {
	menu := BuildTrayMenu(stubController{}, nil, false)
	if item := menu.ItemByID(itemCopyToken); item != nil {
		t.Fatal("tray menu must omit 复制管理 Token when admin auth is disabled")
	}
}

func TestBuildTrayMenu_IncludesTokenItem_WhenAuthEnabled(t *testing.T) {
	menu := BuildTrayMenu(stubController{}, nil, true)
	if item := menu.ItemByID(itemCopyToken); item == nil {
		t.Fatal("tray menu must keep 复制管理 Token when admin auth is enabled")
	}
}

func TestBuildAppMenu_OmitsTokenItem_WhenAuthDisabled(t *testing.T) {
	menu := BuildAppMenu(stubController{}, false)
	if item := menu.ItemByID(itemCopyToken); item != nil {
		t.Fatal("app menu must omit 复制管理 Token when admin auth is disabled")
	}
}

func TestBuildAppMenu_IncludesTokenItem_WhenAuthEnabled(t *testing.T) {
	menu := BuildAppMenu(stubController{}, true)
	if item := menu.ItemByID(itemCopyToken); item == nil {
		t.Fatal("app menu must keep 复制管理 Token when admin auth is enabled")
	}
}
