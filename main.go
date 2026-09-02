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

// main starts the app: it opens the settings file, wires up the three services
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

	app := application.New(application.Options{
		Name:        "k8sdockside",
		Description: "A Kubernetes workspace for your local kubeconfig contexts",
		Services: []application.Service{
			application.NewService(configs),
			application.NewService(NewSettingsService(settings)),
			application.NewService(NewResourceService(configs)),
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
			InvisibleTitleBarHeight: 50,
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
