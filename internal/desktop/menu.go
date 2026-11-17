package desktop

import (
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
}

// Menu item IDs used for dynamic updates.
const (
	itemToggleServe = "toggle-serve"
	itemRebuild     = "rebuild"
	itemCopyToken   = "copy-token"
)

// BuildTrayMenu assembles the tray menu; recents are the recent site paths.
func BuildTrayMenu(c Controller, recents []string) *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "打开预览窗口", ID: "show-window", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
		{Label: "启动/停止预览服务", ID: itemToggleServe, Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
		{Label: "一键构建站点", ID: itemRebuild, Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
		{Label: "复制管理 Token", ID: itemCopyToken, Click: func(*mygo.MenuItem, *mygo.Window) { c.CopyAdminToken() }},
		mygo.Separator(),
		{Label: "打开站点目录", ID: "open-folder", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
		{Label: "在浏览器打开站点", ID: "open-browser", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		{Label: "打开日志目录", ID: "open-logs", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenLogs() }},
		mygo.Separator(),
	}
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
func BuildAppMenu(c Controller) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "站点", Submenu: []*mygo.MenuItem{
			{Label: "打开预览窗口", Click: func(*mygo.MenuItem, *mygo.Window) { c.ShowWindow() }},
			{Label: "启动/停止预览服务", Click: func(*mygo.MenuItem, *mygo.Window) { c.ToggleServe() }},
			{Label: "一键构建站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.Rebuild() }},
			{Label: "复制管理 Token", Click: func(*mygo.MenuItem, *mygo.Window) { c.CopyAdminToken() }},
			mygo.Separator(),
			{Label: "打开站点目录", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteFolder() }},
			{Label: "在浏览器打开站点", Click: func(*mygo.MenuItem, *mygo.Window) { c.OpenSiteInBrowser() }},
		}},
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
func BuildTray(c Controller, recents []string) (*MenuSet, error) {
	trayMenu := BuildTrayMenu(c, recents)
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayPNG,
		IconIsTemplate: true,
		ToolTip:        "huan",
		Menu:           trayMenu,
	})
	if err != nil {
		return nil, err
	}
	appMenu := BuildAppMenu(c)
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

// RefreshRecents rebuilds the tray menu (recent list changed).
func (m *MenuSet) RefreshRecents(c Controller, recents []string) error {
	menu := BuildTrayMenu(c, recents)
	m.TrayMenu = menu
	m.Tray.SetMenu(menu)
	return nil
}
