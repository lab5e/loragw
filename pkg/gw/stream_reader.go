package gw

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/lab5e/lospan/pkg/lg"
	"github.com/lab5e/lospan/pkg/pb/lospan"
	"github.com/lab5e/span/pkg/pb/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const keepAliveInterval = time.Second * 60
const loraClientTimeout = time.Second * 1

// StreamProcessor reads and writes to and from a command stream from the Span service and keeps the LoRa
// service in sync with the commands. The stream is closed when there's an error
type StreamProcessor struct {
	Stream        gateway.UserGateway_ControlStreamClient
	Lora          lospan.LospanClient
	sendChan      chan *gateway.ControlRequest
	lastMessage   time.Time
	deviceMapping map[string]string // map device id => eui
	application   *lospan.Application
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(stream gateway.UserGateway_ControlStreamClient, lora lospan.LospanClient) *StreamProcessor {
	return &StreamProcessor{
		Stream:        stream,
		Lora:          lora,
		sendChan:      make(chan *gateway.ControlRequest),
		lastMessage:   time.Now(),
		deviceMapping: make(map[string]string),
	}
}

// Run runs the stream processor. It will not return unless an error occurs
func (sp *StreamProcessor) Run() {
	receiveCh := make(chan *gateway.ControlResponse)
	errorCh := make(chan error)
	go func() {
		for msg := range sp.sendChan {
			err := sp.Stream.Send(msg)
			if err != nil {
				errorCh <- err
				return
			}
			sp.lastMessage = time.Now()
		}
	}()

	go func() {
		defer close(receiveCh)
		for {
			msg, err := sp.Stream.Recv()
			if err != nil {
				errorCh <- err
				return
			}
			receiveCh <- msg
			sp.lastMessage = time.Now()
		}
	}()

	// Send a Connect message to get the updated configuration
	sp.sendResponse(&gateway.ControlRequest{
		Msg: &gateway.ControlRequest_Config{},
	})
	for {
		select {
		case res := <-receiveCh:
			switch msg := res.Msg.(type) {
			case *gateway.ControlResponse_KeepaliveResponse:
				break

			case *gateway.ControlResponse_GatewayUpdate:
				sp.updateConfig(msg.GatewayUpdate)

			case *gateway.ControlResponse_DeviceRemoved:
				sp.removeDevice(msg.DeviceRemoved)

			case *gateway.ControlResponse_DeviceUpdate:
				sp.updateDevice(msg.DeviceUpdate)

			case *gateway.ControlResponse_DownstreamMessage:
				sp.downstreamMessage(msg.DownstreamMessage)

			default:
				lg.Warning("Unknown message from server: %T", res.Msg)
			}

		case err := <-errorCh:
			close(sp.sendChan)
			lg.Error("Got error when sending/receiving: %v", err)
			return

		case <-time.After(10 * time.Second):
			// Check for timeout, send keepalive if time is > keepAliveInterval
			if time.Since(sp.lastMessage) > keepAliveInterval {
				sp.sendResponse(&gateway.ControlRequest{
					Msg: &gateway.ControlRequest_Keepalive{},
				})
			}
		}
	}
}

func (sp *StreamProcessor) sendResponse(msg *gateway.ControlRequest) {
	sp.sendChan <- msg
}

func (sp *StreamProcessor) updateConfig(msg *gateway.GatewayConfigUpdate) {
	lg.Info("Update gateway config")
	if msg.Config == nil {
		lg.Error("Gateway configuration is empty")
		return
	}
	appEUI, ok := msg.Config["appEui"]
	if !ok {
		lg.Error("Gateway config does not contain App EUI %+v", msg.Config)
		return
	}

	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	var err error
	sp.application, err = sp.Lora.GetApplication(ctx, &lospan.GetApplicationRequest{
		Eui: appEUI,
	})

	if err != nil {
		if status.Code(err) == codes.NotFound {
			// create a new application
			createCtx, createDone := context.WithTimeout(context.Background(), loraClientTimeout)
			defer createDone()
			lg.Info("Application %s not found. Creating a new application", appEUI)
			sp.application, err = sp.Lora.CreateApplication(createCtx, &lospan.CreateApplicationRequest{
				Eui: &appEUI,
			})
			if err != nil {
				lg.Error("Error creating application: %v", err)
				return
			}
			lg.Info("Created application with EUI %s", sp.application.Eui)
			return
		}
		lg.Error("Error retrieving application: %v", err)
		return
	}
	lg.Info("Found application %s in LoRa server. No updates needed", sp.application.Eui)
}

// updateDevice updates the device in the lora application.
func (sp *StreamProcessor) updateDevice(msg *gateway.DeviceConfigUpdate) {
	updatedDevice, err := sp.configToDevice(sp.application, msg.Config)
	if err != nil {
		lg.Error("Error converting config to device: %v", err)
		return
	}
	existing := false
	existingEui, ok := sp.deviceMapping[msg.DeviceId]
	if ok {
		// check if device exists in lora server
		ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
		defer done()

		// if the EUI is omitted we must use the existing mapping.
		_, err = sp.Lora.GetDevice(ctx, &lospan.GetDeviceRequest{
			Eui: existingEui,
		})
		if err != nil && status.Code(err) != codes.NotFound {
			lg.Error("Could not retrieve device %s: %v", msg.DeviceId, err)
			return
		}
		existing = (err == nil)
	}
	updatedDevice.ApplicationEui = &sp.application.Eui
	if !existing {
		ctxCreate, doneCreate := context.WithTimeout(context.Background(), loraClientTimeout)
		defer doneCreate()
		device, err := sp.Lora.CreateDevice(ctxCreate, updatedDevice)
		if err != nil {
			lg.Error("Error creating device for ID %s: %v", msg.DeviceId, err)
			return
		}
		sp.deviceMapping[msg.DeviceId] = *device.Eui
		lg.Info("Created device %s (ID: %s) for application %s", *device.Eui, msg.DeviceId, *device.ApplicationEui)
		return
	}
	if sp.application == nil {
		lg.Error("No application exists yet. Can't create device")
		return
	}

	updatedDevice.Eui = &existingEui
	updateCtx, updateDone := context.WithTimeout(context.Background(), loraClientTimeout)
	defer updateDone()
	device, err := sp.Lora.UpdateDevice(updateCtx, updatedDevice)
	if err != nil {
		lg.Error("Error updating device %s: %v", msg.DeviceId, err)
		return
	}
	sp.deviceMapping[msg.DeviceId] = *device.Eui
	lg.Info("Updated device %s for device ID %s", *device.Eui, msg.DeviceId)
}

func (sp *StreamProcessor) removeDevice(msg *gateway.DeviceRemoved) {
	lg.Info("Remove device")
}

func (sp *StreamProcessor) downstreamMessage(msg *gateway.DownstreamMessage) {
	lg.Info("Downstream message")
}

func (sp *StreamProcessor) configToDevice(app *lospan.Application, cfg map[string]string) (*lospan.Device, error) {
	lg.Debug("Config for device: %+v", cfg)
	ret := &lospan.Device{}
	devEUI, ok := cfg["devEui"]
	if ok {
		ret.Eui = &devEUI
	}
	state, ok := cfg["state"]
	if ok {
		switch state {
		case "otaa":
			ret.State = lospan.DeviceState_OTAA.Enum()
		case "abp":
			ret.State = lospan.DeviceState_ABP.Enum()
		case "disabled":
			ret.State = lospan.DeviceState_DISABLED.Enum()
		default:
			return nil, errors.New("unknown state for device")
		}
	}
	devAddr, ok := cfg["devAddr"]
	if ok && devAddr != "" {
		intAddr, err := strconv.ParseInt(devAddr, 16, 32)
		if err != nil {
			return nil, errors.New("invalid devAddr format")
		}
		p := uint32(intAddr)
		ret.DevAddr = &p
	}
	appKey, ok := cfg["appKey"]
	if ok && appKey != "" {
		buf, err := hex.DecodeString(appKey)
		if err != nil {
			return nil, errors.New("invalid format for appKey")
		}
		ret.AppKey = buf
	}
	appSKey, ok := cfg["appSKey"]
	if ok && appSKey != "" {
		buf, err := hex.DecodeString(appSKey)
		if err != nil {
			return nil, errors.New("invalid format for appSKey")
		}
		ret.AppSessionKey = buf
	}
	nwkSKey, ok := cfg["nwkSKey"]
	if ok && nwkSKey != "" {
		buf, err := hex.DecodeString(nwkSKey)
		if err != nil {
			return nil, errors.New("invalid format for nwkSKey")
		}
		ret.NetworkSessionKey = buf
	}
	fcntUp, ok := cfg["fCntUp"]
	if ok && fcntUp != "" {
		fup, err := strconv.ParseInt(fcntUp, 10, 32)
		if err != nil {
			return nil, errors.New("invalid fCntUp format")
		}
		p := int32(fup)
		ret.FrameCountUp = &p
	}
	fcntDn, ok := cfg["fCntDn"]
	if ok && fcntDn != "" {
		fdn, err := strconv.ParseInt(fcntDn, 10, 32)
		if err != nil {
			return nil, errors.New("invalid fCntDn format")
		}
		p := int32(fdn)
		ret.FrameCountDown = &p
	}
	relaxedCounter, ok := cfg["relaxedCounter"]
	if ok && relaxedCounter != "" {
		rc := false
		if relaxedCounter == "true" {
			rc = true
		}
		ret.RelaxedCounter = &rc
	}
	return ret, nil
}
