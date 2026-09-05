package main

import (
	"embed"
	"log"

	"github.com/roger/k8sdockside/internal/appconfig"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// main starts the app: it opens the settings file, wires up the eight services
// the frontend calls, and shows the main window.
func main() {
	// The settings store holds the user's kubeconfig paths, context aliases and
	// colours. A failure here means we could not read an existing settings file,
	// and carrying on would silently discard the user's customisation.
	settings, err := appconfig.Open()
	if err != nil {
		log.Fatalf("k8sdockside: %v", err)
	}

	configs := NewKubeconfigService(settings)
	// The action service borrows the resource service's watcher rather than
	// opening its own: acting on an object in a context already showing in a
	// tab should cost no second connection and no second credential exec.
	resources := NewResourceService(configs)
	// The plugin service borrows the same watcher, so a plugin's overview
	// counts through the connection its tabs are already using rather than
	// opening a second one. The two know about each other because a tab opened
	// on a plugin's view has to be resolved back to a real kind -- see
	// ResourceService.view.
	solutions := NewPluginService(settings, configs, resources.watcher)
	resources.usePlugins(solutions)
	// Charts go through the same watcher again: a Prometheus query reaches the
	// cluster through the API server, so it rides the connection a tab already
	// has rather than opening its own.
	graphs := NewMetricsService(settings, configs, resources.watcher, solutions)

	app := application.New(application.Options{
		Name:        "k8sdockside",
		Description: "A Kubernetes workspace for your local kubeconfig contexts",
		Services: []application.Service{
			application.NewService(configs),
			application.NewService(NewSettingsService(settings)),
			application.NewService(resources),
			application.NewService(NewActionService(configs, resources.watcher)),
			application.NewService(NewLogService(configs, resources.watcher)),
			application.NewService(NewThemeService(settings)),
			application.NewService(solutions),
			application.NewService(graphs),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "k8sdockside",
		// Wide enough for the sidebar, a tab bar and a docked detail panel
		// side by side without anything collapsing.
		Width:     1440,
		Height:    900,
		MinWidth:  960,
		MinHeight: 600,
		Mac: application.MacWindow{
			// Matches the height of the frontend's own top bar, so the traffic
			// lights sit centred in it rather than over the content below.
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(0x0f, 0x13, 0x1a),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
