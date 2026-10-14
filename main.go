package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"agentprovidermanager/internal/app"
	"agentprovidermanager/internal/core"
	appupdate "agentprovidermanager/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

var (
	// 版本号唯一真源为仓库根目录的 VERSION 文件，经 scripts/sync-version.mjs
	// 生成到 internal/core/version_gen.go，这里只做引用，避免多处硬编码漂移。
	version   = core.AppVersion
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
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if len(os.Args) > 1 && os.Args[1] == appupdate.CleanupBackupFlag {
		if err := appupdate.RunBackupCleanup(cleanupCtx, os.Args[1:], ""); err != nil {
			fmt.Fprintf(os.Stderr, "Update backup cleanup deferred: %v\n", err)
		}
	}
	if err := appupdate.CleanupStaleBackups(cleanupCtx, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Stale update backup cleanup deferred: %v\n", err)
	}
	cleanupCancel()
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
