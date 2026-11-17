package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/iannil/huan/internal/observability"
)

// App wires the MyGo lifecycle, the serve runtime, menus, and persistence
// into the desktop shell.
type App struct {
	mu     sync.Mutex
	cfg    *DesktopConfig
	cfgDir string
	rt     *ServeRuntime
	win    *mygo.Window
	menus  *MenuSet
	log    *observability.Logger
	logDir string
}

// Run starts the desktop app and blocks until it quits.
func Run() error {
	app := &App{}

	// Structured JSON log file (Full-Lifecycle Observability).
	dir, err := DefaultConfigDir()
	if err == nil {
		app.logDir = filepath.Join(dir, "logs")
		_ = os.MkdirAll(app.logDir, 0o755)
		f, ferr := os.OpenFile(filepath.Join(app.logDir, "desktop.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr == nil {
			app.log = observability.NewLoggerWithWriter("", io.MultiWriter(f, os.Stderr))
		}
	}
	if app.log == nil {
		app.log = observability.NewLogger("")
	}

	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	app.cfgDir = filepath.Dir(cfgPath)
	cfg, err := LoadOrDefault(cfgPath)
	if err != nil {
		return err
	}
	app.cfg = cfg

	mygo.Bind(DesktopBindings{app: app})
	// Keep the app alive in the tray when the last window closes: the MyGo
	// default quits without a listener here (common macOS convention is to
	// stay resident). ShowWindow recreates the window on demand.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(app.onReady)
	return mygo.App.Run()
}

// currentSourceDir returns the active site directory ("" if none).
func (a *App) currentSourceDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rt == nil || !a.rt.Running() {
		if len(a.cfg.RecentSites) > 0 {
			return a.cfg.RecentSites[0]
		}
		return ""
	}
	return a.rt.SourceDir()
}

// notify shows a best-effort system notification; denial is not fatal.
func (a *App) notify(title, body string) {
	n := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
	if err := n.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
		a.log.LogError("notify", err, nil)
	}
}

// onReady runs once the MyGo app is ready: tray, site, server, window.
func (a *App) onReady() {
	menus, err := BuildTray(a, a.cfg.RecentSites)
	if err != nil {
		a.log.LogError("tray", err, nil)
	}
	a.mu.Lock()
	a.menus = menus
	a.mu.Unlock()

	// Ensure a site: use most-recent, else ask. Invalid entries fall through
	// to the picker with an explanatory error dialog.
	site := ""
	if len(a.cfg.RecentSites) > 0 {
		if err := ValidateSite(a.cfg.RecentSites[0]); err == nil {
			site = a.cfg.RecentSites[0]
		} else {
			mygo.Dialog.Error("站点不可用", err.Error())
		}
	}
	if site == "" {
		s, ok := a.pickSiteDialog()
		if !ok {
			a.log.Log("onReady", observability.EventBranch, map[string]any{"branch": "no-site-selected"})
			return // stay in tray; user can pick via menu
		}
		site = s
	}
	a.startAndServe(site)
	a.showWindow()
}

// pickSiteDialog loops the directory picker until a valid site or cancel.
func (a *App) pickSiteDialog() (string, bool) {
	for {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     "选择 huan 站点目录",
			Directory: true,
		})
		if err != nil {
			a.log.LogError("pick-site", err, nil)
			return "", false
		}
		if len(paths) == 0 || paths[0] == "" {
			return "", false // user canceled
		}
		if err := ValidateSite(paths[0]); err != nil {
			mygo.Dialog.Error("不是有效的 huan 站点", err.Error())
			continue
		}
		return paths[0], true
	}
}

// startAndServe starts (or restarts) the server for the given site and
// reloads the window. Called from the ready flow and site switches.
func (a *App) startAndServe(site string) {
	startLog := a.log.LogFunctionStart("start-serve", map[string]any{"site": site})
	startTime := time.Now()
	if a.rt != nil {
		_ = a.rt.Stop(5 * time.Second)
	}
	rt := NewServeRuntime(ServeOptions{
		SourceDir: site,
		Logf: func(format string, args ...any) {
			a.log.Log("serve", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)})
		},
	})
	rt.OnServeExited = func(err error) {
		// Intentional stop (nil / context.Canceled): update UI quietly.
		// Unexpected exit: also notify the user (spec §6).
		if err == nil || errors.Is(err, context.Canceled) {
			if a.menus != nil {
				a.menus.SetServing(false)
			}
			a.showOffline()
			return
		}
		a.log.LogError("serve-exited", err, nil)
		a.notify("huan 预览服务已停止", "可通过托盘菜单重新启动")
		if a.menus != nil {
			a.menus.SetServing(false)
		}
		a.showOffline()
	}
	rt.OnRebuildFailed = func(err error) {
		a.notify("重建失败", err.Error())
	}
	adminURL, err := rt.Start()
	if err != nil {
		a.log.LogError(startLog, err, nil)
		mygo.Dialog.Error("预览服务启动失败", err.Error())
		if a.menus != nil {
			a.menus.SetServing(false)
		}
		return
	}
	a.mu.Lock()
	a.rt = rt
	a.cfg.AddRecentSite(site)
	a.mu.Unlock()
	if err := Save(filepath.Join(a.cfgDir, "config.json"), a.cfg); err != nil {
		a.log.LogError("save-config", err, nil)
	}
	if a.menus != nil {
		a.menus.SetServing(true)
		a.menus.RefreshRecents(a, a.cfg.RecentSites)
	}
	a.log.LogFunctionEnd(startLog, time.Since(startTime), map[string]any{"admin_url": adminURL})
	a.loadAdmin(adminURL)
}

// ShowWindow implements Controller: create or focus the admin window.
// No blocking work, so it runs on the menu callback thread directly.
func (a *App) ShowWindow() { a.showWindow() }

// showWindow creates or focuses the admin window.
func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		return // Phase 1: single window; created once in onReady flow
	}
	newWin := mygo.NewWindow(mygo.WindowOptions{
		Title:  "huan",
		Width:  1280,
		Height: 800,
		// TrustedOrigins lets the admin SPA origin call Go bindings.
		Page: mygo.PageOptions{TrustedOrigins: []string{"http://127.0.0.1"}},
	})
	a.mu.Lock()
	a.win = newWin
	a.mu.Unlock()
	// Window close (Cmd+W) must clear a.win so ShowWindow can recreate it.
	// OnClosed fires after the window is truly gone (an OnClose listener
	// would clear the reference even if some other listener kept the window
	// open). Also runs during app quit; clearing then is harmless.
	newWin.OnClosed(func() {
		a.mu.Lock()
		a.win = nil
		a.mu.Unlock()
	})
	url := ""
	if a.rt != nil {
		url = a.rt.AdminURL()
	}
	if url != "" {
		if err := newWin.Page().LoadURL(url); err != nil {
			a.log.LogError("load-admin", err, nil)
		}
	}
}

// loadAdmin points the window at the given admin URL (or an offline page).
func (a *App) loadAdmin(adminURL string) {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	if adminURL == "" {
		a.showOffline()
		return
	}
	if err := win.Page().LoadURL(adminURL); err != nil {
		a.log.LogError("load-admin", err, nil)
	}
}

// showOffline renders a minimal local page when the server is stopped.
func (a *App) showOffline() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win == nil {
		return
	}
	win.Page().LoadHTML(
		`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><title>huan</title></head>
<body style="font-family:-apple-system,sans-serif;display:grid;place-items:center;height:100vh;margin:0">
<div style="text-align:center"><h1>预览服务已停止</h1><p>通过菜单栏托盘图标可重新启动预览服务。</p></div>
</body></html>`, "")
}

// ---- Controller implementation (menu callbacks run on the main thread;
// blocking work goes to goroutines, per MyGo dialog docs) ----

func (a *App) ToggleServe() {
	go func() {
		if a.rt != nil && a.rt.Running() {
			if err := a.rt.Stop(5 * time.Second); err != nil {
				mygo.Dialog.Error("停止失败", err.Error())
			}
			if a.menus != nil {
				a.menus.SetServing(false)
			}
			a.showOffline()
			return
		}
		site := a.currentSourceDir()
		if site == "" {
			a.ChooseSite()
			return
		}
		a.startAndServe(site)
	}()
}

func (a *App) Rebuild() {
	go func() {
		rt := a.rt
		if rt == nil || !rt.Running() {
			mygo.Dialog.Error("无法构建", "预览服务未运行，请先选择站点")
			return
		}
		if a.menus != nil {
			a.menus.SetBuilding(true)
		}
		cfg := rt.Config()
		outDir := filepath.Join(rt.SourceDir(), cfg.PublishDir)
		err := BuildSiteTo(rt.SourceDir(), outDir, func(format string, args ...any) {
			a.log.Log("build", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)})
		})
		if a.menus != nil {
			a.menus.SetBuilding(false)
		}
		if err != nil {
			a.log.LogError("build", err, map[string]any{"out": outDir})
			a.notify("构建失败", err.Error())
			return
		}
		a.notify("构建完成", "输出目录："+outDir)
	}()
}

func (a *App) OpenSiteFolder() {
	if dir := a.currentSourceDir(); dir != "" {
		mygo.Shell.ShowItemInFolder(dir)
	}
}

func (a *App) OpenSiteInBrowser() {
	if a.rt != nil && a.rt.SiteURL() != "" {
		mygo.Shell.OpenExternal(a.rt.SiteURL())
	}
}

func (a *App) SwitchSite(path string) {
	go func() {
		if err := ValidateSite(path); err != nil {
			mygo.Dialog.Error("站点不可用", err.Error())
			return
		}
		a.startAndServe(path)
	}()
}

func (a *App) ChooseSite() {
	go func() {
		site, ok := a.pickSiteDialog()
		if !ok {
			return
		}
		a.startAndServe(site)
	}()
}

func (a *App) OpenLogs() {
	if a.logDir != "" {
		mygo.Shell.ShowItemInFolder(a.logDir)
	}
}

func (a *App) Quit() {
	go func() {
		if a.rt != nil {
			_ = a.rt.Stop(5 * time.Second)
		}
		// App.Quit confirmed present (go doc); the brief's os.Exit(0) was a
		// fallback for its absence only, and would race the graceful quit.
		mygo.App.Quit()
	}()
}

// Compile-time check: App satisfies the Controller contract the menus use.
var _ Controller = (*App)(nil)
