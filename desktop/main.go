// Command clawdyX is the desktop control panel for the clawdyX worker.
// The worker (a separate launchd job) owns the Telegram bot, Claude jobs,
// the token and captures; this app talks to it over its private socket.
package main

import (
	"embed"
	_ "embed"
	"log"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/tray-template.png
var trayIcon []byte

var version = "0.3.3"

func main() {
	worker := &WorkerService{}
	system := &SystemService{w: worker}
	app := application.New(application.Options{
		Name:        "clawdyX",
		Description: "Control panel for the clawdyX Telegram bot and the local Claude Code CLI",
		Services: []application.Service{
			application.NewService(worker),
			application.NewService(system),
			application.NewService(&SetupService{}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			// Closing the window keeps the app (and its menu-bar item) alive.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{UniqueID: "io.github.publicthrone547.clawdyx.app"},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "clawdyX",
		Width:     1100,
		Height:    720,
		MinWidth:  860,
		MinHeight: 560,
		URL:       "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 38,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(17, 18, 22),
	})
	// Close hides the window; the worker is unaffected either way.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
		app.Event.Emit("app:hidden", true)
	})
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		window.Show()
		window.Focus()
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("clawdyX")
	menu := application.NewMenu()
	stateItem := menu.Add("Worker: checking…").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Open clawdyX").OnClick(func(*application.Context) { window.Show(); window.Focus() })
	toggle := menu.Add("Stop bot")
	toggle.OnClick(func(*application.Context) {
		if st, err := worker.Status(); err == nil {
			if st.Bot.State == "polling" || st.Bot.State == "paused" {
				_, _ = worker.BotStop(false)
			} else {
				_, _ = worker.BotStart()
			}
		}
	})
	menu.AddSeparator()
	menu.Add("Quit clawdyX").OnClick(func(*application.Context) { confirmQuit(app, worker) })
	tray.SetMenu(menu)

	update := func() {
		st, err := worker.Status()
		switch {
		case err != nil:
			stateItem.SetLabel("Worker: not running")
			toggle.SetLabel("Start bot").SetEnabled(false)
			tray.SetLabel("")
		case st.Bot.State == "polling" || st.Bot.State == "paused":
			stateItem.SetLabel("Bot: " + st.Bot.State + jobsSuffix(st.Jobs.Running))
			toggle.SetLabel("Stop bot").SetEnabled(true)
			tray.SetLabel("")
		default:
			stateItem.SetLabel("Bot: " + st.Bot.State)
			toggle.SetLabel("Start bot").SetEnabled(true)
			tray.SetLabel("off")
		}
		menu.Update()
	}
	app.Event.On("worker:connection", func(*application.CustomEvent) { update() })
	app.Event.On("worker:event", func(e *application.CustomEvent) {
		if ev, ok := e.Data.(WorkerEvent); ok && (ev.Type == "bot.state" || strings.HasPrefix(ev.Type, "job.")) && ev.Type != "job.progress" {
			update()
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func jobsSuffix(n int) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return " · 1 job running"
	}
	return " · " + itoa(n) + " jobs running"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// confirmQuit asks before quitting while the bot polls; the worker keeps
// running unless the user chooses to stop the bot too.
func confirmQuit(app *application.App, worker *WorkerService) {
	st, err := worker.Status()
	if err != nil || (st.Bot.State != "polling" && st.Bot.State != "paused") {
		app.Quit()
		return
	}
	d := app.Dialog.Question().SetTitle("Quit clawdyX?").
		SetMessage("The background worker keeps the Telegram bot running after you quit the app. You can also stop the bot now.")
	d.AddButton("Quit app").OnClick(func() { app.Quit() }).SetAsDefault()
	d.AddButton("Quit and stop bot").OnClick(func() {
		_, _ = worker.BotStop(false)
		app.Quit()
	})
	d.AddButton("Cancel").SetAsCancel()
	d.Show()
}
