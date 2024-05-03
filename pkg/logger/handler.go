package logger

import (
	"log/slog"

	"github.com/lab5e/spangw/pkg/gw"
)

// New creates a new logging command handler
func New(ch gw.CommandHandler) gw.CommandHandler {
	return &logger{h: ch}
}

type logger struct {
	h gw.CommandHandler
}

func (l *logger) UpdateConfig(localID string, config map[string]string) (string, error) {
	slog.Debug("UpdateConfig In", "id", localID, "config", config)
	ret, err := l.h.UpdateConfig(localID, config)
	slog.Debug("UpdateConfig Out", "id", ret, "error", err)
	return ret, err
}

func (l *logger) RemoveDevice(localID string, deviceID string) error {
	slog.Debug("RemoveDevice In", "id", localID, "deviceID", deviceID)
	err := l.h.RemoveDevice(localID, deviceID)
	slog.Debug("RemoveDevice Out", "error", err)
	return err
}

func (l *logger) UpdateDevice(localID string, localDeviceID string, config map[string]string) (string, map[string]string, error) {
	slog.Debug("in", localID, "localID", localDeviceID, "config")
	ret, res, err := l.h.UpdateDevice(localID, localDeviceID, config)
	slog.Debug("Out: ID: %s  config: %+v  err: %v", ret, res, err)
	return ret, res, err
}

func (l *logger) DownstreamMessage(localID, localDeviceID, messageID string, payload []byte) error {
	slog.Debug("Downstream In", "id", localID, "deviceID", localDeviceID, "messageID", messageID, "payloadLen", len(payload))
	err := l.h.DownstreamMessage(localID, localDeviceID, messageID, payload)
	slog.Debug("Downstream Out", "error", err)
	return err
}

func (l *logger) UpstreamMessage(upstreamCb gw.UpstreamMessageFunc) {
	l.h.UpstreamMessage(upstreamCb)
}

func (l *logger) Shutdown() {
}
