// Package main implements a lora gateway to Span
package main

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/loragw/pkg/gw"
	"github.com/lab5e/loragw/pkg/logger"
	"github.com/lab5e/loragw/pkg/lora"
	"github.com/lab5e/lospan/pkg/server"
)

type param struct {
	Gateway gw.Parameters     `kong:"embed"`
	Lora    server.Parameters `kong:"embed,prefix='lora-',help='LoRa server options'"`
}

func main() {
	var config param
	kong.Parse(&config)

	loraHandler, err := lora.New(&config.Lora)
	if err != nil {
		lg.Error("Error creating LoRa server: %v", err)
		os.Exit(2)
	}
	gwHandler, err := gw.Create(config.Gateway, logger.New(loraHandler))
	if err != nil {
		lg.Error("Error creating gateway: %v", err)
		os.Exit(2)
	}

	defer gwHandler.Stop()
	if err := gwHandler.Run(); err != nil {
		lg.Error("Could not run the gateway process: %v", err)
		os.Exit(2)
	}
}
