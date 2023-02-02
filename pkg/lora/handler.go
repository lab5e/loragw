package lora

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/loragw/pkg/gw"
	"github.com/lab5e/lospan/pkg/congress"
	"github.com/lab5e/lospan/pkg/pb/lospan"
	"github.com/lab5e/lospan/pkg/server"
	"github.com/lab5e/span/pkg/gateways/usergw/gwconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const loraClientTimeout = time.Second * 1

// New creates a new LoRa command handler for the gateway command processor
func New(config *server.Parameters) (gw.CommandHandler, error) {
	ret := &loraHandler{
		mutex: &sync.Mutex{},
	}
	var err error
	ret.loraServer, err = congress.NewLoRaServer(config)
	if err != nil {
		return nil, err
	}
	if err := ret.loraServer.Start(); err != nil {
		return nil, err
	}

	cc, err := grpc.Dial(addrToEndpoint(ret.loraServer.ListenAddress()),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	ret.loraClient = lospan.NewLospanClient(cc)

	return ret, nil
}

type loraHandler struct {
	loraServer       *congress.LoRaServer
	loraClient       lospan.LospanClient
	upstreamCallback gw.UpstreamMessageFunc
	mutex            *sync.Mutex
}

func (l *loraHandler) Shutdown() {
	l.loraServer.Shutdown()
}

func (l *loraHandler) UpdateConfig(localID string, config map[string]string) (string, error) {
	appEUI := config[gwconfig.LoraApplicationEUI]
	// Configuration should contain the application EUI
	if appEUI == "" {
		return "", errors.New("missing application EUI from configuration")
	}

	if localID == "" {
		// This is a new application. Add it to the server
		createCtx, createDone := context.WithTimeout(context.Background(), loraClientTimeout)
		defer createDone()
		app, err := l.loraClient.CreateApplication(createCtx, &lospan.CreateApplicationRequest{
			Eui: &appEUI,
		})
		if err != nil {
			return "", err
		}
		go l.createUpstreamReader(appEUI)
		return app.Eui, nil
	}
	// Nothing to update
	go l.createUpstreamReader(appEUI)
	return localID, nil
}

func (l *loraHandler) RemoveDevice(localID string, deviceID string) error {
	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	_, err := l.loraClient.DeleteDevice(ctx, &lospan.DeleteDeviceRequest{
		Eui: localID,
	})
	if err != nil {
		lg.Warning("Device %s does not exist; not removed: %v", localID, err)
	}
	return nil
}

func (l *loraHandler) UpdateDevice(localID string, localDeviceID string, config map[string]string) (string, map[string]string, error) {
	if localDeviceID == "" {
		return l.createDevice(localID, localDeviceID, config)
	}
	return l.updateDevice(localID, localDeviceID, config)
}

func (l *loraHandler) DownstreamMessage(localID, localDeviceID, messageID string, payload []byte) error {
	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	if localID == "" {
		return errors.New("can't send downstream message with no EUI")
	}
	_, err := l.loraClient.SendMessage(ctx, &lospan.DownstreamMessage{
		Eui:     localID,
		Payload: payload,
		Port:    42,    // TODO: Allow port to be set via protocol
		Ack:     false, // TODO: use ack flag for message
	})

	return err
}

func (l *loraHandler) UpstreamMessage(upstreamCb gw.UpstreamMessageFunc) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.upstreamCallback = upstreamCb
}

func (l *loraHandler) createDevice(appEUI string, deviceEUI string, config map[string]string) (string, map[string]string, error) {
	if appEUI == "" {
		return deviceEUI, nil, errors.New("application EUI is not set; cant create")
	}

	devEUI := config[gwconfig.LoraDeviceEUI]
	newDevice := &lospan.Device{
		ApplicationEui: &appEUI,
	}

	// If the device EUI is omitted we'll generate one for you
	if devEUI != "" {
		newDevice.Eui = &devEUI
	}
	if err := l.configToDevice(newDevice, config); err != nil {
		return deviceEUI, nil, err
	}

	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()
	createdDevice, err := l.loraClient.CreateDevice(ctx, newDevice)
	if err != nil {
		return deviceEUI, nil, err
	}
	l.deviceToConfig(createdDevice, config)
	return *createdDevice.Eui, config, nil
}

func (l *loraHandler) updateDevice(appEUI string, deviceEUI string, config map[string]string) (string, map[string]string, error) {
	if appEUI == "" {
		return deviceEUI, nil, errors.New("application EUI not set; cant update")
	}
	if deviceEUI == "" {
		return deviceEUI, nil, errors.New("device EUI not set; cant update")
	}
	ctx, done := context.WithTimeout(context.Background(), loraClientTimeout)
	defer done()

	device := &lospan.Device{
		Eui:            &deviceEUI,
		ApplicationEui: &appEUI,
	}
	if err := l.configToDevice(device, config); err != nil {
		return deviceEUI, nil, err
	}

	updatedDevice, err := l.loraClient.UpdateDevice(ctx, device)
	if err != nil {
		return deviceEUI, nil, err
	}
	l.deviceToConfig(updatedDevice, config)
	return *device.Eui, config, nil
}

func (l *loraHandler) configToDevice(device *lospan.Device, cfg map[string]string) error {
	state, ok := cfg[gwconfig.LoraState]
	if ok {
		switch state {
		case "otaa":
			device.State = lospan.DeviceState_OTAA.Enum()
		case "abp":
			device.State = lospan.DeviceState_ABP.Enum()
		case "disabled":
			device.State = lospan.DeviceState_DISABLED.Enum()
		default:
			return errors.New("unknown state for device")
		}
	}
	if device.State == lospan.DeviceState_OTAA.Enum() {
		appKey, ok := cfg[gwconfig.LoraAppKey]
		if ok && appKey != "" {
			buf, err := hex.DecodeString(appKey)
			if err != nil {
				return errors.New("invalid format for appKey")
			}
			device.AppKey = buf
		}
	}
	if device.State == lospan.DeviceState_ABP.Enum() {
		devAddr, ok := cfg[gwconfig.LoraDevAddr]
		if ok && devAddr != "" {
			intAddr, err := strconv.ParseInt(devAddr, 16, 32)
			if err != nil {
				return errors.New("invalid devAddr format")
			}
			p := uint32(intAddr)
			device.DevAddr = &p
		}

		appSKey, ok := cfg[gwconfig.LoraAppSKey]
		if ok && appSKey != "" {
			buf, err := hex.DecodeString(appSKey)
			if err != nil {
				return errors.New("invalid format for appSKey")
			}
			device.AppSessionKey = buf
		}
		nwkSKey, ok := cfg[gwconfig.LoraNwkSKey]
		if ok && nwkSKey != "" {
			buf, err := hex.DecodeString(nwkSKey)
			if err != nil {
				return errors.New("invalid format for nwkSKey")
			}
			device.NetworkSessionKey = buf
		}
	}
	fcntUp, ok := cfg[gwconfig.LoraFCntUp]
	if ok && fcntUp != "" {
		fup, err := strconv.ParseInt(fcntUp, 10, 32)
		if err != nil {
			return errors.New("invalid fCntUp format")
		}
		p := int32(fup)
		device.FrameCountUp = &p
	}
	fcntDn, ok := cfg[gwconfig.LoraFCntDn]
	if ok && fcntDn != "" {
		fdn, err := strconv.ParseInt(fcntDn, 10, 32)
		if err != nil {
			return errors.New("invalid fCntDn format")
		}
		p := int32(fdn)
		device.FrameCountDown = &p
	}
	relaxedCounter, ok := cfg[gwconfig.LoraRelaxedCounter]
	if ok && relaxedCounter != "" {
		rc := false
		if relaxedCounter == "true" {
			rc = true
		}
		device.RelaxedCounter = &rc
	}
	return nil
}

func (l *loraHandler) deviceToConfig(device *lospan.Device, config map[string]string) {
	config[gwconfig.LoraApplicationEUI] = device.GetApplicationEui()
	switch device.GetState() {
	case lospan.DeviceState_ABP:
		config[gwconfig.LoraState] = "abp"
	case lospan.DeviceState_OTAA:
		config[gwconfig.LoraState] = "otaa"
	default:
		config[gwconfig.LoraState] = "disabled"
	}
	if device.DevAddr != nil {
		config[gwconfig.LoraDevAddr] = strconv.FormatInt(int64(*device.DevAddr), 16)
	}
	if len(device.AppKey) > 0 {
		config[gwconfig.LoraAppKey] = hex.EncodeToString(device.AppKey)
	}
	if len(device.AppSessionKey) > 0 {
		config[gwconfig.LoraAppSKey] = hex.EncodeToString(device.AppSessionKey)
	}
	if len(device.NetworkSessionKey) > 0 {
		config[gwconfig.LoraNwkSKey] = hex.EncodeToString(device.NetworkSessionKey)
	}
	if device.FrameCountDown != nil {
		config[gwconfig.LoraFCntDn] = strconv.FormatInt(int64(*device.FrameCountDown), 10)
	}
	if device.FrameCountUp != nil {
		config[gwconfig.LoraFCntUp] = strconv.FormatInt(int64(*device.FrameCountUp), 10)
	}
	if device.RelaxedCounter != nil {
		if *device.RelaxedCounter {
			config[gwconfig.LoraRelaxedCounter] = "true"
		} else {
			config[gwconfig.LoraRelaxedCounter] = "false"
		}
	}
}

// Create a stream reader for upstream messages from the LoRa devices. If there's an error reading the stream
// it will stop. This might be an issue
func (l *loraHandler) createUpstreamReader(appEUI string) {
	if appEUI == "" {
		return
	}
	ctx := context.Background()
	streamClient, err := l.loraClient.StreamMessages(ctx, &lospan.StreamMessagesRequest{
		Eui: appEUI,
	})
	if err != nil {
		lg.Warning("Error opening upstream message stream for app %s: %v", appEUI, err)
	}
	defer streamClient.CloseSend()
	for {
		msg, err := streamClient.Recv()
		if err != nil {
			lg.Warning("Error reading upstream messages for app %s. Exiting: %v", appEUI, err)
			return
		}
		var upstreamCB gw.UpstreamMessageFunc
		l.mutex.Lock()
		upstreamCB = l.upstreamCallback
		l.mutex.Unlock()
		if l.upstreamCallback != nil {
			metadata := make(map[string]string)
			metadata[gwconfig.LoraGatewayEUI] = msg.GatewayEui
			metadata[gwconfig.LoraRSSI] = strconv.FormatInt(int64(msg.Rssi), 10)
			metadata[gwconfig.LoraSNR] = fmt.Sprintf("%3.2f", msg.Snr)
			metadata[gwconfig.LoraFrequency] = fmt.Sprintf("%5.3f", msg.Snr)
			metadata[gwconfig.LoraDataRate] = msg.DataRate
			metadata[gwconfig.LoraDevAddr] = strconv.FormatInt(int64(msg.DevAddr), 16)
			upstreamCB(msg.Eui, msg.Payload, metadata)
		}
	}
}
