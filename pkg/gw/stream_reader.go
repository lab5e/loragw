package gw

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lab5e/lospan/pkg/lg"
	"github.com/lab5e/lospan/pkg/pb/lospan"
	"github.com/lab5e/span/pkg/gateways/usergw/gwconfig"
	"github.com/lab5e/span/pkg/pb/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const keepAliveInterval = time.Second * 60
const loraClientTimeout = time.Second * 1

// StreamProcessor reads and writes to and from a command stream from the Span service and keeps the LoRa
// service in sync with the commands. The stream is closed when there's an error
type StreamProcessor struct {
	Stream          gateway.UserGateway_ControlStreamClient
	Lora            lospan.LospanClient
	sendChan        chan *gateway.ControlRequest
	deviceMapping   map[string]string // map device id => eui
	loraApplication *lospan.Application
	closeUpstreamCh chan bool // Channel for upstream messages
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(stream gateway.UserGateway_ControlStreamClient, lora lospan.LospanClient) *StreamProcessor {
	return &StreamProcessor{
		Stream:          stream,
		Lora:            lora,
		sendChan:        make(chan *gateway.ControlRequest),
		deviceMapping:   make(map[string]string),
		closeUpstreamCh: make(chan bool, 1),
	}
}

// Run runs the stream processor. It will not return unless an error occurs
func (sp *StreamProcessor) Run() {
	receiveCh := make(chan *gateway.ControlResponse)
	errorCh := make(chan error)
	lastMessage := time.Now()

	go func() {
		for msg := range sp.sendChan {
			err := sp.Stream.Send(msg)
			if err != nil {
				errorCh <- err
				return
			}
			lastMessage = time.Now()
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
			lastMessage = time.Now()
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
			if time.Since(lastMessage) > keepAliveInterval {
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
	appEUI, ok := msg.Config[gwconfig.LoraApplicationEUI]
	if !ok {
		lg.Error("Gateway config does not contain App EUI %+v", msg.Config)
		return
	}

	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	var err error
	app, err := sp.Lora.GetApplication(ctx, &lospan.GetApplicationRequest{
		Eui: appEUI,
	})

	if err != nil {
		if status.Code(err) == codes.NotFound {
			// create a new application
			createCtx, createDone := context.WithTimeout(context.Background(), loraClientTimeout)
			defer createDone()
			lg.Info("Application %s not found. Creating a new application", appEUI)
			app, err = sp.Lora.CreateApplication(createCtx, &lospan.CreateApplicationRequest{
				Eui: &appEUI,
			})
			if err != nil {
				lg.Error("Error creating application: %v", err)
				return
			}
			lg.Info("Created application with EUI %s", appEUI)
			sp.setLoraApplication(app)
			return
		}
		lg.Error("Error retrieving application: %v", err)
		return
	}
	lg.Info("Found application %s in LoRa server. No updates needed", app.Eui)
}

// updateDevice updates the device in the lora application.
func (sp *StreamProcessor) updateDevice(msg *gateway.DeviceConfigUpdate) {
	app := sp.getLoraApplication()
	if app == nil {
		lg.Error("No application is set. Ignoring device %s", msg.DeviceId)
		return
	}
	updatedDevice, err := sp.configToDevice(app, msg.Config)
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
	updatedDevice.ApplicationEui = &app.Eui
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
	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	eui, ok := sp.deviceMapping[msg.DeviceId]
	if !ok {
		lg.Error("Unknown device %s - ignoring downstream message", msg.DeviceId)
		return
	}
	downMsg, err := sp.Lora.SendMessage(ctx, &lospan.DownstreamMessage{
		Eui:     eui,
		Payload: msg.Payload,
		Port:    42,    // TODO: Allow port to be set via protocol
		Ack:     false, // TODO: use ack flag for message
	})
	if err != nil {
		lg.Error("Error sending downstream message to %s: %v", msg.DeviceId, err)
		return
	}

	lg.Info("Sent downstream message to device %s ", downMsg.Eui)
}

func (sp *StreamProcessor) configToDevice(app *lospan.Application, cfg map[string]string) (*lospan.Device, error) {
	lg.Debug("Config for device: %+v", cfg)
	ret := &lospan.Device{}
	devEUI, ok := cfg[gwconfig.LoraDeviceEUI]
	if ok {
		ret.Eui = &devEUI
	}
	state, ok := cfg[gwconfig.LoraState]
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

	if ret.State == lospan.DeviceState_ABP.Enum() {
		devAddr, ok := cfg[gwconfig.LoraDevAddr]
		if ok && devAddr != "" {
			intAddr, err := strconv.ParseInt(devAddr, 16, 32)
			if err != nil {
				return nil, errors.New("invalid devAddr format")
			}
			p := uint32(intAddr)
			ret.DevAddr = &p
		}
		appKey, ok := cfg[gwconfig.LoraAppKey]
		if ok && appKey != "" {
			buf, err := hex.DecodeString(appKey)
			if err != nil {
				return nil, errors.New("invalid format for appKey")
			}
			ret.AppKey = buf
		}
		appSKey, ok := cfg[gwconfig.LoraAppSKey]
		if ok && appSKey != "" {
			buf, err := hex.DecodeString(appSKey)
			if err != nil {
				return nil, errors.New("invalid format for appSKey")
			}
			ret.AppSessionKey = buf
		}
		nwkSKey, ok := cfg[gwconfig.LoraNwkSKey]
		if ok && nwkSKey != "" {
			buf, err := hex.DecodeString(nwkSKey)
			if err != nil {
				return nil, errors.New("invalid format for nwkSKey")
			}
			ret.NetworkSessionKey = buf
		}
	}
	fcntUp, ok := cfg[gwconfig.LoraFCntUp]
	if ok && fcntUp != "" {
		fup, err := strconv.ParseInt(fcntUp, 10, 32)
		if err != nil {
			return nil, errors.New("invalid fCntUp format")
		}
		p := int32(fup)
		ret.FrameCountUp = &p
	}
	fcntDn, ok := cfg[gwconfig.LoraFCntDn]
	if ok && fcntDn != "" {
		fdn, err := strconv.ParseInt(fcntDn, 10, 32)
		if err != nil {
			return nil, errors.New("invalid fCntDn format")
		}
		p := int32(fdn)
		ret.FrameCountDown = &p
	}
	relaxedCounter, ok := cfg[gwconfig.LoraRelaxedCounter]
	if ok && relaxedCounter != "" {
		rc := false
		if relaxedCounter == "true" {
			rc = true
		}
		ret.RelaxedCounter = &rc
	}
	return ret, nil
}

func (sp *StreamProcessor) getLoraApplication() *lospan.Application {
	return sp.loraApplication
}

func (sp *StreamProcessor) setLoraApplication(app *lospan.Application) {
	// Stop the old
	sp.closeUpstreamCh <- true
	oldCh := sp.closeUpstreamCh
	sp.loraApplication = app
	sp.closeUpstreamCh = make(chan bool, 1)
	close(oldCh)
	go sp.readUpstreamMessages()
}

// Read upstream channel messages and feed them into the gateway's
func (sp *StreamProcessor) readUpstreamMessages() {
	app := sp.getLoraApplication()
	if app == nil {
		lg.Error("Can't stream messages from nil application")
		return
	}
	lg.Info("Starting upstream reader for application %s", app.Eui)
	ctx := context.Background()
	streamClient, err := sp.Lora.StreamMessages(ctx, &lospan.StreamMessagesRequest{
		Eui: app.Eui,
	})

	if err != nil {
		lg.Error("Error streaming upstream messages from application %s: %v", app.Eui, err)
		return
	}
	upstreamCh := make(chan *lospan.UpstreamMessage)
	defer close(upstreamCh)

	defer streamClient.CloseSend()
	go func() {
		for {
			msg, err := streamClient.Recv()
			if err != nil {
				lg.Warning("Error receiving upstream message. Exiting: %v", err)
				return
			}
			upstreamCh <- msg
		}
	}()

	for {
		select {
		case <-sp.closeUpstreamCh:
			return
		case msg := <-upstreamCh:
			deviceID, err := sp.getIDForEUI(msg.Eui)
			if err != nil {
				lg.Warning("Uknown device EUI: %s. Ignoring upstream message (%+v)", msg.Eui, sp.deviceMapping)
				continue
			}
			lg.Info("Send UpstreamMessage for device %s", deviceID)
			sp.sendResponse(&gateway.ControlRequest{
				Msg: &gateway.ControlRequest_UpstreamMessage{
					UpstreamMessage: &gateway.UpstreamMessage{
						DeviceId: deviceID,
						Payload:  msg.Payload,
						Metadata: sp.makeUpstreamMetadata(msg),
					},
				},
			})

			updateInfo := &gateway.ControlRequest_DeviceUpdate{
				DeviceUpdate: &gateway.DeviceUpdate{
					DeviceId: deviceID,
					Config:   make(map[string]string),
					Metadata: make(map[string]string),
				},
			}

			md := updateInfo.DeviceUpdate.Metadata
			md[gwconfig.LoraRSSI] = strconv.FormatInt(int64(msg.Rssi), 10)
			md[gwconfig.LoraSNR] = fmt.Sprintf("%3.2f", msg.Snr)
			md[gwconfig.LoraFrequency] = fmt.Sprintf("%3.2f", msg.Frequency)
			md[gwconfig.LoraDataRate] = msg.DataRate
			md[gwconfig.LoraGatewayEUI] = msg.GatewayEui

			loraDevice, err := sp.Lora.GetDevice(ctx, &lospan.GetDeviceRequest{
				Eui: msg.Eui,
			})
			if err != nil {
				lg.Warning("Got error retrieving device %s. Won't update config: %v", msg.Eui, err)
				continue
			}
			cfg := updateInfo.DeviceUpdate.Config
			cfg[gwconfig.LoraApplicationEUI] = *loraDevice.ApplicationEui
			cfg[gwconfig.LoraDeviceEUI] = *loraDevice.Eui
			cfg[gwconfig.LoraAppKey] = hex.EncodeToString(loraDevice.AppKey)
			cfg[gwconfig.LoraAppSKey] = hex.EncodeToString(loraDevice.AppSessionKey)
			cfg[gwconfig.LoraNwkSKey] = hex.EncodeToString(loraDevice.NetworkSessionKey)
			if loraDevice.FrameCountDown != nil {
				cfg[gwconfig.LoraFCntDn] = strconv.FormatInt(int64(*loraDevice.FrameCountDown), 10)
			}
			if loraDevice.FrameCountUp != nil {
				cfg[gwconfig.LoraFCntUp] = strconv.FormatInt(int64(*loraDevice.FrameCountUp), 10)
			}
			if loraDevice.KeyWarning != nil {
				if *loraDevice.KeyWarning {
					cfg[gwconfig.LoraKeyWarning] = "true"
				}
			}
			if loraDevice.DevAddr != nil {
				cfg[gwconfig.LoraDevAddr] = strconv.FormatInt(int64(*loraDevice.DevAddr), 16)
			}
			sp.sendResponse(&gateway.ControlRequest{
				Msg: updateInfo,
			})
		}
	}
}

func (sp *StreamProcessor) makeUpstreamMetadata(msg *lospan.UpstreamMessage) map[string]string {
	ret := make(map[string]string)
	ret[gwconfig.LoraGatewayEUI] = msg.GatewayEui
	ret[gwconfig.LoraRSSI] = strconv.FormatInt(int64(msg.Rssi), 10)
	ret[gwconfig.LoraSNR] = fmt.Sprintf("%3.2f", msg.Snr)
	ret[gwconfig.LoraFrequency] = fmt.Sprintf("%5.3f", msg.Snr)
	ret[gwconfig.LoraDataRate] = msg.DataRate
	ret[gwconfig.LoraDevAddr] = strconv.FormatInt(int64(msg.DevAddr), 16)
	return ret
}

func (sp *StreamProcessor) getIDForEUI(deviceEUI string) (string, error) {
	for id, eui := range sp.deviceMapping {
		if eui == deviceEUI {
			return id, nil
		}
	}
	return "", errors.New("not found")
}
