package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/iannil/huan/internal/observability"
	"github.com/iannil/huan/internal/plugin"
)

// App wires the MyGo lifecycle, the serve runtime, menus, and persistence
// into the desktop shell.
type App struct {
	mu     sync.Mutex
	cfg    *DesktopConfig
	cfgDir string
	rt     *ServeRuntime
	sm     *Machine // spec §5 state model: concurrency gate + transition truth
	// startMu serializes startAndServe: the 4-state machine cannot
	// distinguish "serving stably" from "serving = start in flight", so
	// mutual exclusion of starts is what makes machine==Serving on entry
	// mean a stable session (restart path) rather than a concurrent start.
	startMu sync.Mutex
	win     *mygo.Window
	menus   *MenuSet
	// diag collects startup problems (plugin load warnings, build issues);
	// created in Run before any goroutine can Add into it.
	diag   *Diagnostics
	log    *observability.Logger
	logDir string
}

// Run starts the desktop app and blocks until it quits.
func Run() error {
	app := &App{}
	app.sm = NewMachine()
	app.diag = NewDiagnostics()

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
	// Serialize starts (see startMu doc): guarantees a single in-flight start.
	a.startMu.Lock()
	defer a.startMu.Unlock()

	startLog := a.log.LogFunctionStart("start-serve", map[string]any{"site": site})
	startTime := time.Now()
	// Spec §5 state gate: claim the machine for this whole operation.
	//   idle → serving            fresh start
	//   serving → idle → serving  stable session restart (site switch) —
	//                             parking through idle releases the serving
	//                             slot so concurrent stops/claims stay honest
	//   building → idle → serving serve restart while a production build runs
	//                             (the build's completion CAS then loses,
	//                             harmlessly — see Rebuild)
	//   error → idle              spec §6 error recovery, then fresh start
	// Losing any claim means another holder owns the machine — merge instead
	// of duplicating (repeat-click idempotence).
	if a.sm.Current() == StateError {
		a.sm.Transition(StateError, StateIdle)
	}
	cur := a.sm.Current()
	switch cur {
	case StateIdle:
		if !a.sm.Transition(StateIdle, StateServing) {
			a.log.Log("start-serve", observability.EventBranch, map[string]any{
				"branch": "state-gate-lost", "current": a.sm.Current().String(), "site": site,
			})
			return
		}
	case StateServing, StateBuilding:
		if !a.sm.Transition(cur, StateIdle) || !a.sm.Transition(StateIdle, StateServing) {
			a.log.Log("start-serve", observability.EventBranch, map[string]any{
				"branch": "state-gate-lost", "current": a.sm.Current().String(), "site": site,
			})
			return
		}
	}
	// Detach the old runtime before stopping it so its stale OnServeExited
	// (identity check below) cannot downgrade the UI mid-restart.
	a.mu.Lock()
	old := a.rt
	a.rt = nil
	a.mu.Unlock()
	if old != nil {
		_ = old.Stop(5 * time.Second)
	}
	rt := NewServeRuntime(ServeOptions{
		SourceDir: site,
		Logger:    a.log, // serve-start/serve-stop spans land in desktop.log
		Logf: func(format string, args ...any) {
			a.log.Log("serve", observability.EventPoint, map[string]any{"msg": fmt.Sprintf(format, args...)})
		},
	})
	this := rt
	rt.OnServeExited = func(err error) {
		// Stale-callback guard: only the live (a.rt-identical) session may
		// drive UI; callbacks from a replaced runtime are dropped.
		a.mu.Lock()
		live := a.rt == this
		a.mu.Unlock()
		if !live {
			a.log.Log("serve-exited", observability.EventBranch, map[string]any{"branch": "stale-exit-skipped"})
			return
		}
		// Machine CAS decides who downgrades: serving→idle is the normal
		// stop/exit; building→idle covers a serve death mid-build (still
		// downgrade — the server really exited); anything else is stale.
		switch {
		case a.sm.Transition(StateServing, StateIdle):
		case a.sm.Transition(StateBuilding, StateIdle):
			a.log.Log("serve-exited", observability.EventBranch, map[string]any{"branch": "serve-exited-during-building"})
		default:
			a.log.Log("serve-exited", observability.EventBranch, map[string]any{
				"branch": "unexpected-state-skip", "current": a.sm.Current().String(),
			})
			return
		}
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
		// Start failed: release the gate back to idle (spec §6 error→idle
		// recovery collapsed into one step; the retry entry point is the same
		// idle→serving path).
		a.sm.Transition(StateServing, StateIdle)
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
	// TrustedOrigins are fixed at window creation and every Start binds a new
	// port, so each successful start rebuilds the window for the fresh origin.
	a.recreateWindow()
}

// ShowWindow implements Controller: create or focus the admin window.
// No blocking work, so it runs on the menu callback thread directly.
func (a *App) ShowWindow() { a.showWindow() }

// adminOrigin derives the TrustedOrigins entry for the admin SPA from the
// running server's admin URL. mygo's isTrusted compares scheme://host:port
// exactly (u.Host includes the port), and the dev server binds a fresh
// random port per Start, so the origin must be derived at window-creation
// time — a bare "http://127.0.0.1" can never match. Nil when there is no
// usable admin URL (offline window: nothing to trust).
func adminOrigin(adminURL string) []string {
	u, err := url.Parse(adminURL)
	if err != nil || u.Host == "" {
		return nil
	}
	return []string{u.Scheme + "://" + u.Host}
}

// showWindow creates the admin window if none exists (menu entry point).
func (a *App) showWindow() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil {
		return // one window per serve session; startAndServe rebuilds it
	}
	a.createWindow()
}

// createWindow builds the admin window with TrustedOrigins derived from the
// running runtime (empty when none — the offline page needs no bindings).
func (a *App) createWindow() {
	a.mu.Lock()
	rt := a.rt
	a.mu.Unlock()
	var origins []string
	adminURL := ""
	if rt != nil {
		adminURL = rt.AdminURL()
		origins = adminOrigin(adminURL)
	}
	newWin := mygo.NewWindow(mygo.WindowOptions{
		Title:  "huan",
		Width:  1280,
		Height: 800,
		// TrustedOrigins lets the admin SPA origin call Go bindings; the
		// origin carries the per-session port (see adminOrigin).
		Page: mygo.PageOptions{TrustedOrigins: origins},
	})
	a.mu.Lock()
	a.win = newWin
	a.mu.Unlock()
	// Window close (Cmd+W) must clear a.win so showWindow can recreate it.
	// OnClosed fires after the window is truly gone. The identity check keeps
	// a stale OnClosed (from a window recreated mid-close) from clobbering
	// the reference to the new window; clearing during app quit is harmless.
	newWin.OnClosed(func() {
		a.mu.Lock()
		if a.win == newWin {
			a.win = nil
		}
		a.mu.Unlock()
	})
	if adminURL != "" {
		if err := newWin.Page().LoadURL(adminURL); err != nil {
			a.log.LogError("load-admin", err, nil)
		}
	}
}

// recreateWindow closes the current window (if any) and creates a fresh one
// bound to the new runtime's origin, loading the admin URL. WindowOptions
// fix TrustedOrigins at creation and every Start binds a new port, so a
// site switch or serve restart must rebuild the window for bindings to work.
func (a *App) recreateWindow() {
	a.mu.Lock()
	old := a.win
	a.mu.Unlock()
	if old != nil {
		old.Close() // fires the old window's OnClosed; identity check guards
	}
	a.createWindow()
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
		a.mu.Lock()
		rt := a.rt
		a.mu.Unlock()
		if rt != nil && rt.Running() {
			// Machine gate (spec §5): stopping downgrades only from serving.
			// Losing the CAS means a start/restart holds the machine or a
			// build is in flight — don't stop out from under it and skip the
			// offline-page/SetServing(false) UI downgrade.
			if !a.sm.Transition(StateServing, StateIdle) {
				a.log.Log("toggle-serve", observability.EventBranch, map[string]any{
					"branch": "state-gate-lost", "current": a.sm.Current().String(),
				})
				return
			}
			if err := rt.Stop(5 * time.Second); err != nil {
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
		a.mu.Lock()
		rt := a.rt
		a.mu.Unlock()
		// Machine gate (spec §5): building is reachable only from serving.
		// Running() stays as a guard because the machine holds serving during
		// the start-up window too (before the listener answers) — building
		// then would deref an unready runtime; the CAS serializes concurrent
		// builds and stop-mid-build.
		if rt == nil || !rt.Running() || !a.sm.Transition(StateServing, StateBuilding) {
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
		}, func(ws []plugin.LoadWarning) {
			for _, w := range ws {
				msg := fmt.Sprintf("%s: %v", w.Name, w.Err)
				a.diag.Add(LevelWarn, "build", msg)
				a.log.Log("diag", observability.EventPoint, map[string]any{"component": "build", "msg": msg})
			}
			a.refreshDiagMenu()
		})
		// Back to serving only if the machine still says building; if the
		// serve exited mid-build or a restart claimed the machine meanwhile
		// (OnServeExited / startAndServe moved building→idle), the CAS loses
		// and we keep the machine where its new owner put it.
		if !a.sm.Transition(StateBuilding, StateServing) {
			a.log.Log("build", observability.EventBranch, map[string]any{
				"branch": "build-return-transition-lost", "current": a.sm.Current().String(),
			})
		}
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

// CopyAdminToken implements Controller: copy the live admin API token to the
// clipboard so the SPA's auth prompt can be satisfied in a Finder-launched
// .app (env/stderr are unreachable there). No persistence — the token is
// regenerated per serve session and cleared on Stop.
func (a *App) CopyAdminToken() {
	go func() {
		a.mu.Lock()
		rt := a.rt
		a.mu.Unlock()
		token := ""
		if rt != nil {
			token = rt.Token()
		}
		if token == "" {
			mygo.Dialog.Error("预览服务未运行", "启动预览服务后再复制管理 Token")
			return
		}
		mygo.Clipboard.WriteText(token)
		a.notify("已复制管理 Token", "请粘贴到管理界面的认证提示中")
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

// ShowDiagnostics opens the startup-diagnostics dialog (wired in full in the
// app-integration task).
func (a *App) ShowDiagnostics() { a.showDiagnostics() }

func (a *App) showDiagnostics() {}

// refreshDiagMenu updates the startup-diagnostics badge in both menus from
// the current snapshot size. Safe to call from any goroutine (Add callbacks,
// build workers); a no-op before the menus exist or during early startup.
func (a *App) refreshDiagMenu() {
	a.mu.Lock()
	menus := a.menus
	a.mu.Unlock()
	if menus == nil || a.diag == nil {
		return
	}
	menus.SetDiagCount(len(a.diag.Snapshot()))
}

// Compile-time check: App satisfies the Controller contract the menus use.
var _ Controller = (*App)(nil)
