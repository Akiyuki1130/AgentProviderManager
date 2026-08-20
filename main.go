package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"agentprovidermanager/internal/app"
)

var (
	version   = "1.0.1"
	buildTime = "unknown"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("AgentProviderManager %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	appBinding := app.NewApp()

	err := wails.Run(&options.App{
		Title:     fmt.Sprintf("Agent Provider Manager v%s", version),
		Width:     1280,
		Height:    800,
		MinWidth:  1100,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 243, G: 243, B: 243, A: 1},
		OnStartup:  appBinding.Startup,
		OnShutdown: appBinding.Shutdown,
		Bind: []interface{}{
			appBinding,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}
}
