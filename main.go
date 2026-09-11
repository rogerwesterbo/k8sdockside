package main

import (
	"embed"
	"log"

	"github.com/rogerwesterbo/k8sdockside/internal/appconfig"
	"github.com/rogerwesterbo/k8sdockside/internal/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

// main starts the app: it opens the settings file, registers the services the
// frontend calls -- built and wired in internal/services -- and shows the
// main window.
func main() {
	// The settings store holds the user's kubeconfig paths, context aliases and
	// colours. A failure here means we could not read an existing settings file,
	// and carrying on would silently discard the user's customisation.
	settings, err := appconfig.Open()
	if err != nil {
		log.Fatalf("k8sdockside: %v", err)
	}

	registered, pluginViews := services.New(settings)

	app := application.New(application.Options{
		Name: "K8s Dockside",
		// Wails renders Name as the title and Description as the body of the
		// About dialog under the app menu, and uses Description nowhere else,
		// so the version goes here to be seen there.
		Description: "A Kubernetes workspace for your local kubeconfig contexts\n\nVersion " + services.DisplayVersion(),
		Services:    registered,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// Serves plugins' own views from their folders, and refuses those
			// views' sandboxed frames any direct call into the services above.
			Middleware: pluginViews,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "K8s Dockside",
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
