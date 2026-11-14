package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	frontend, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	app := NewApp()
	err = wails.Run(&options.App{
		Title:            "Rizzbase Studio",
		Width:            1280,
		Height:           820,
		MinWidth:         900,
		MinHeight:        620,
		AssetServer:      &assetserver.Options{Assets: frontend},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind:             []interface{}{app},
		BackgroundColour: options.NewRGBA(247, 243, 232, 1),
	})
	if err != nil {
		log.Fatal(err)
	}
}
