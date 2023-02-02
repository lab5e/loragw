package logger

import (
	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/loragw/pkg/gw"
)

func New(ch gw.CommandHandler) gw.CommandHandler {
	return &logger{h: ch}
}

type logger struct {
	h gw.CommandHandler
}

func (l *logger) UpdateConfig(localID string, config map[string]string) (string, error) {
	lg.Info("In: ID: %s  config=%+v", localID, config)
	ret, err := l.h.UpdateConfig(localID, config)
	lg.Info("Out: ID: %s: error: %v", ret, err)
	return ret, err
}

func (l *logger) RemoveDevice(localID string, deviceID string) error {
	lg.Info("In: ID: %s  deviceID: %s", localID, deviceID)
	err := l.h.RemoveDevice(localID, deviceID)
	lg.Info("Out: error: %v", err)
	return err
}

func (l *logger) UpdateDevice(localID string, localDeviceID string, config map[string]string) (string, map[string]string, error) {
	lg.Info("In: ID: %s,  LocalID: %s   config=%+v", localID, localDeviceID, config)
	ret, res, err := l.h.UpdateDevice(localID, localDeviceID, config)
	lg.Info("Out: ID: %s  config: %+v  err: %v", ret, res, err)
	return ret, res, err
}

func (l *logger) DownstreamMessage(localID, localDeviceID, messageID string, payload []byte) error {
	lg.Info("In: ID: %s  deviceID: %s  messageID: %s  payload: %d bytes", localID, localDeviceID, messageID, len(payload))
	err := l.h.DownstreamMessage(localID, localDeviceID, messageID, payload)
	lg.Info("Out:  error: %v", err)
	return err
}

func (l *logger) UpstreamMessage(upstreamCb gw.UpstreamMessageFunc) {
	l.h.UpstreamMessage(upstreamCb)
}

func (l *logger) Shutdown() {
	l.h.Shutdown()
}
