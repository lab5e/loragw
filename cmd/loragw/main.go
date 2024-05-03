// Package main implements a lora gateway to Span
package main

import (
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/lab5e/loragw/pkg/logger"
	"github.com/lab5e/loragw/pkg/lora"
	"github.com/lab5e/lospan/pkg/server"
	"github.com/lab5e/spangw/pkg/gw"
)

type param struct {
	Gateway     gw.Parameters     `kong:"embed"`
	Lora        server.Parameters `kong:"embed,prefix='lora-',help='LoRa server options'"`
	MessagePort uint8             `kong:"help='LoRaWAN port for downstream messages',default='1'"`
}

func main() {
	var config param
	kong.Parse(&config)

	loraHandler, err := lora.New(&config.Lora, config.MessagePort)
	if err != nil {
		slog.Error("Error creating LoRa server", "error", err)
		return
	}
	defer loraHandler.Shutdown()
	gwHandler, err := gw.Create(config.Gateway, logger.New(loraHandler))
	if err != nil {
		slog.Error("Error creating gateway", "error", err)
		return
	}

	defer gwHandler.Stop()
	if err := gwHandler.Run(); err != nil {
		slog.Error("Could not run the gateway process", "error", err)
		return
	}
}
