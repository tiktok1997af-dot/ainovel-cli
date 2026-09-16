package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// frontendAssets contains the production Vite build.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// GUI-02B.2 binds the typed gateway now, but intentionally does not create
	// or open a project-scoped AppRuntime. GUI-02C owns project lifecycle and
	// will provide the runtime authority later. Until then, every call fails
	// closed with a typed runtime_unavailable AppError.
	gateway := newGateway(nil, emitWailsDesktopEvent)

	err := wails.Run(&options.App{
		Title:             "AINOVEL Desktop",
		Width:             1600,
		Height:            980,
		MinWidth:          1180,
		MinHeight:         720,
		DisableResize:     false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour: &options.RGBA{
			R: 9,
			G: 14,
			B: 21,
			A: 1,
		},
		AssetServer: &assetserver.Options{Assets: frontendAssets},
		OnStartup:   gateway.startup,
		OnShutdown:  gateway.shutdown,
		Bind:        []interface{}{gateway},
	})
	if err != nil {
		log.Fatal(err)
	}
}
