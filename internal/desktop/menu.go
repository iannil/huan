package desktop

import (
	"fmt"

	"github.com/egoist/mygo"
)

// Controller abstracts the app-level actions the menus trigger. It keeps
// menu wiring testable and decoupled from the MyGo app lifecycle (Task 8
// implements it on top of ServeRuntime).
type Controller interface {
	ToggleServe()       // start/stop the in-process server
	Rebuild()           // one-click production build
	CopyAdminToken()    // copy the current admin API token to the clipboard
	OpenSiteFolder()    // reveal site root in Finder
	OpenSiteInBrowser() // open siteURL in default browser
	SwitchSite(path string)
	ChooseSite() // open directory picker and switch
	OpenLogs()   // reveal log directory in Finder
	ShowWindow() // create/focus the admin window
	Quit()
	ShowDiagnostics() // open the startup-diagnostics dialog
}

// Menu item IDs used for dynamic updates.
const (
	itemToggleServe = "toggle-serve"
	itemRebuild     = "rebuild"
	itemCopyToken   = "copy-token"
	itemDiag        = "startup-diag"
)

// BuildTrayMenu assembles the tray menu; recents are the recent site paths.
// adminAuthRequired=false (admin.requireToken: false) omits the copy-token
// item — there is no token to copy.
func BuildTrayMenu(c Controller, recents []string, adminAuthRequired bool) *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "打开预览窗口", ID: "show-window", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
		{Label: "启动/停止预览服务", ID: itemToggleServe, Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
		{Label: "一键构建站点", ID: itemRebuild, Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
	}
	if adminAuthRequired {
		items = append(items, &mygo.MenuItem{Label: "复制管理 Token", ID: itemCopyToken, Click: func(*mygo.MenuItem, *mygo.Window) { c.CopyAdminToken() }})
	}
	items = append(items,
		&mygo.MenuItem{Label: diagMenuLabel(0), ID: itemDiag, Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowDiagnostics() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "打开站点目录", ID: "open-folder", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
		&mygo.MenuItem{Label: "在浏览器打开站点", ID: "open-browser", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		&mygo.MenuItem{Label: "打开日志目录", ID: "open-logs", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenLogs() }},
		mygo.Separator(),
	)
	if len(recents) > 0 {
		subs := make([]*mygo.MenuItem, 0, len(recents)+1)
		for _, p := range recents {
			p := p
			subs = append(subs, &mygo.MenuItem{Label: p, Click: func(*mygo.MenuItem, *mygo.Window) { c.SwitchSite(p) }})
		}
		subs = append(subs, &mygo.MenuItem{Label: "选择其他目录…", ID: "choose-site", Click: func(*mygo.MenuItem, *mygo.Window) { c.ChooseSite() }})
		items = append(items, &mygo.MenuItem{Label: "切换站点", Type: mygo.MenuItemSubmenu, Submenu: subs})
	} else {
		items = append(items, &mygo.MenuItem{Label: "选择站点目录…", ID: "choose-site", Click: func(*mygo.MenuItem, *mygo.Window) { c.ChooseSite() }})
	}
	items = append(items, mygo.Separator(), &mygo.MenuItem{Label: "退出 huan", ID: "quit", Click: func(*mygo.MenuItem, *mygo.Window) { c.Quit() }})
	return mygo.NewMenu(items)
}

// BuildAppMenu assembles the macOS application menu.
// adminAuthRequired=false (admin.requireToken: false) omits the copy-token
// item — there is no token to copy (same rule as BuildTrayMenu).
func BuildAppMenu(c Controller, adminAuthRequired bool) *mygo.Menu {
	siteItems := []*mygo.MenuItem{
		{Label: "打开预览窗口", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
		{Label: "启动/停止预览服务", Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
		{Label: "一键构建站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
	}
	if adminAuthRequired {
		siteItems = append(siteItems, &mygo.MenuItem{Label: "复制管理 Token", ID: itemCopyToken, Click: func(*mygo.MenuItem, *mygo.Window) { c.CopyAdminToken() }})
	}
	siteItems = append(siteItems,
		&mygo.MenuItem{Label: diagMenuLabel(0), ID: itemDiag, Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowDiagnostics() }},
		mygo.Separator(),
		&mygo.MenuItem{Label: "打开站点目录", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
		&mygo.MenuItem{Label: "在浏览器打开站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
	)
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "站点", Submenu: siteItems},
		{Role: mygo.RoleClose},
		{Role: mygo.RoleQuit},
	})
}

// MenuSet holds the live tray/app menus. Items the app updates dynamically
// are looked up by ID on TrayMenu (see SetServing/SetBuilding).
type MenuSet struct {
	Tray     *mygo.Tray
	AppMenu  *mygo.Menu
	TrayMenu *mygo.Menu
}

// BuildTray creates the tray icon with its menu.
func BuildTray(c Controller, recents []string, adminAuthRequired bool) (*MenuSet, error) {
	trayMenu := BuildTrayMenu(c, recents, adminAuthRequired)
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayPNG,
		IconIsTemplate: true,
		ToolTip:        "huan",
		Menu:           trayMenu,
	})
	if err != nil {
		return nil, err
	}
	appMenu := BuildAppMenu(c, adminAuthRequired)
	mygo.App.SetMenu(appMenu)
	tray.OnClick(func() { c.ShowWindow() })
	return &MenuSet{Tray: tray, AppMenu: appMenu, TrayMenu: trayMenu}, nil
}

// SetServing updates the serve toggle label; native menu items update in place.
func (m *MenuSet) SetServing(serving bool) {
	label := "启动预览服务"
	if serving {
		label = "停止预览服务"
	}
	if item := m.TrayMenu.ItemByID(itemToggleServe); item != nil {
		item.SetLabel(label)
	}
}

// SetBuilding disables the build item while a build is in flight.
func (m *MenuSet) SetBuilding(building bool) {
	if item := m.TrayMenu.ItemByID(itemRebuild); item != nil {
		item.SetEnabled(!building)
	}
}

// diagMenuLabel renders the startup-diagnostics menu title for n items.
func diagMenuLabel(n int) string {
	if n <= 0 {
		return "启动诊断…"
	}
	return fmt.Sprintf("启动诊断（%d 个问题）…", n)
}

// SetDiagCount updates the startup-diagnostics item label in both menus.
func (m *MenuSet) SetDiagCount(n int) {
	label := diagMenuLabel(n)
	if item := m.TrayMenu.ItemByID(itemDiag); item != nil {
		item.SetLabel(label)
	}
	if item := m.AppMenu.ItemByID(itemDiag); item != nil {
		item.SetLabel(label)
	}
}

// RefreshRecents rebuilds both live menus (recent list changed). The
// adminAuthRequired flag re-applies the copy-token visibility rule on every
// rebuild (admin.requireToken can only change with a serve restart, and the
// app menu is built once in onReady before the first Start — when the flag
// is still the true default — so it must be rebuilt here to actually lose
// or regain the copy-token item).
func (m *MenuSet) RefreshRecents(c Controller, recents []string, adminAuthRequired bool) error {
	menu := BuildTrayMenu(c, recents, adminAuthRequired)
	m.TrayMenu = menu
	m.Tray.SetMenu(menu)
	appMenu := BuildAppMenu(c, adminAuthRequired)
	m.AppMenu = appMenu
	// New menus carry itemDiag/itemCopyToken IDs, so SetDiagCount lookups
	// keep working after the swap.
	mygo.App.SetMenu(appMenu)
	return nil
}
