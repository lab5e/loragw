// Package main implements a lora gateway to Span
package main

import (
	"github.com/alecthomas/kong"
	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/loragw/pkg/logger"
	"github.com/lab5e/loragw/pkg/lora"
	"github.com/lab5e/lospan/pkg/server"
	"github.com/lab5e/spangw/pkg/gw"
)

// This will be shown externally so use a stripped-down log config
type logParameters struct {
	Level string `kong:"help='Logging level',default='debug',enum='debug,metrics,info,audit,warning,error'"`
	Type  string `kong:"help='Log type',default='plain',enum='plain,console,json'"`
	Full  bool   `kong:"help='Full log output',default='false'"`
}

type param struct {
	Gateway     gw.Parameters     `kong:"embed"`
	Lora        server.Parameters `kong:"embed,prefix='lora-',help='LoRa server options'"`
	Log         logParameters     `kong:"embed,prefix='log-',help='Log parameters'"`
	MessagePort uint8             `kong:"help='LoRaWAN port for downstream messages',default='1'"`
}

func main() {
	var config param
	kong.Parse(&config)

	logParam := lg.LogParameters{
		Level:        config.Log.Level,
		Type:         config.Log.Type,
		Full:         config.Log.Full,
		MuteStandard: false,
		LiveLogs:     false,
		SlotFile:     false,
	}
	lg.InitLogs("loragw", logParam)

	loraHandler, err := lora.New(&config.Lora, config.MessagePort)
	if err != nil {
		lg.Error("Error creating LoRa server: %v", err)
		return
	}
	defer loraHandler.Shutdown()
	gwHandler, err := gw.Create(config.Gateway, logger.New(loraHandler))
	if err != nil {
		lg.Error("Error creating gateway: %v", err)
		return
	}

	defer gwHandler.Stop()
	if err := gwHandler.Run(); err != nil {
		lg.Error("Could not run the gateway process: %v", err)
		return
	}
}
