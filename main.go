package main

import (
	"embed"
	"log"
	"time"

	"pmanage/pkg/accelerator"
	"pmanage/pkg/alerts"
	"pmanage/pkg/classify"
	"pmanage/pkg/history"
	"pmanage/pkg/metrics"
	"pmanage/pkg/process"
	"pmanage/pkg/servermode"
	"pmanage/pkg/settings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[accelerator.Sample]("telemetry")
	application.RegisterEvent[alerts.Fire]("alert")
}

// serverMiddleware returns the token-gating middleware for server mode, or nil
// for desktop builds so asset serving is unchanged.
func serverMiddleware(cfg *servermode.Config) application.Middleware {
	if cfg == nil {
		return nil
	}
	return cfg.Middleware
}

// serverOptions maps server-mode config onto Wails' headless options.
func serverOptions(cfg *servermode.Config) application.ServerOptions {
	if cfg == nil {
		return application.ServerOptions{}
	}
	return application.ServerOptions{
		Host: cfg.Host,
		Port: cfg.Port,
	}
}

func main() {
	registry := accelerator.NewRegistry()
	nvSource := accelerator.NewNvidiaSource()
	if !nvSource.Detect() {
		registry.AddSource(accelerator.NewNvidiaSmiSource())
	} else {
		registry.AddSource(nvSource)
	}
	for _, s := range accelerator.PlatformSources() {
		registry.AddSource(s)
	}
	for _, s := range accelerator.OptionalSources() {
		if s.Detect() {
			registry.AddSource(s)
		}
	}
	registry.Detect()
	// No accelerator fallback: if nothing real is detected, the app runs honest
	// empty state (N/A chips) rather than fabricating synthetic devices. The mock
	// source remains available for `-tags mock` development builds only.

	svc := metrics.NewService(registry, time.Second)

	hist := history.NewService(time.Second, 1) // 1 hour retention at 1 Hz
	alertEng := alerts.NewEngine()
	procSvc := process.NewService()

	// Feed in-process sinks from the polling loop so history/alerts survive
	// UI restarts and are evaluated in pure Go without round-tripping events.
	svc.AddSink(func(sample accelerator.Sample) {
		hist.Ingest(sample)
		fires := alertEng.Evaluate(sample)
		for _, f := range fires {
			// push active fires to the frontend via the event bus
			if svc.App() != nil {
				svc.App().Event.Emit("alert", f)
			}
		}
	})

	// Server mode (`-tags server`): load token/auth config and start the
	// process service read-only until an admin token is presented. Desktop
	// builds pass a nil config and skip gating.
	var srvCfg *servermode.Config
	if application.System.IsServer() {
		cfg, err := servermode.Load()
		if err != nil {
			log.Fatal(err)
		}
		srvCfg = cfg
		if cfg.ReadOnly() {
			procSvc.SetReadOnly(true)
		}
		log.Printf("server mode: listening on %s (auth=%v, read-only=%v)", cfg.ListenAddr(), cfg.Token != "", cfg.ReadOnly())
	}

	app := application.New(application.Options{
		Name:        "pmanage",
		Description: "GPU / NPU / TPU process viewer and manager",
		Services: []application.Service{
			application.NewService(svc),
			application.NewService(procSvc),
			application.NewService(settings.NewService(registry)),
			application.NewService(hist),
			application.NewService(alertEng),
			application.NewService(classify.NewService()),
			application.NewService(servermode.NewService(srvCfg, procSvc)),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: serverMiddleware(srvCfg),
		},
		Server: serverOptions(srvCfg),
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	svc.SetApp(app)

	if !application.System.IsServer() {
		app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title:  "pmanage",
			Width:  1280,
			Height: 800,
			Mac: application.MacWindow{
				InvisibleTitleBarHeight: 36,
				Backdrop:                application.MacBackdropTranslucent,
				TitleBar:                application.MacTitleBarHiddenInset,
			},
			BackgroundColour: application.NewRGB(9, 9, 11),
			URL:              "/",
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
