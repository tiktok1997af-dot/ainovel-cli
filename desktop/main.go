package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// frontendAssets contains the production Vite build. GUI-02A deliberately
// binds no product authority yet; the shell is renderer/runtime foundation only.
//
//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
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
	})
	if err != nil {
		log.Fatal(err)
	}
}
