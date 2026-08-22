package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"agentprovidermanager/internal/app"
	appupdate "agentprovidermanager/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

var (
	version   = "2.0.1"
	buildTime = "unknown"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == appupdate.HelperFlag {
		if err := appupdate.RunHelper(context.Background(), os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "Update helper failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("AgentProviderManager %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	appBinding := app.NewApp()

	err := wails.Run(&options.App{
		Title:     fmt.Sprintf("Agent Provider Manager v%s", version),
		Width:     1280,
		Height:    800,
		MinWidth:  1120,
		MinHeight: 680,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:         &options.RGBA{R: 243, G: 243, B: 243, A: 1},
		EnableDefaultContextMenu: false,
		OnStartup:                appBinding.Startup,
		OnShutdown:               appBinding.Shutdown,
		Bind: []interface{}{
			appBinding,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}
}
