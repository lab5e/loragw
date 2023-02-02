package gw

// UpstreamMessageFunc is the callback handler for upstream messages. The metadata field contains
// updated device metadata (f.e. RSSI, SNR, frame counters) for the device in Span.
type UpstreamMessageFunc func(localDeviceID string, payload []byte, metadata map[string]string)

// The CommandHandler interface handles the gateway commands
type CommandHandler interface {
	// UpdateConfig updates the gateway configuration. The returned ID is the mapping for the gateway
	// into another ID. On the initial call this ID might be blank
	UpdateConfig(localID string, config map[string]string) (string, error)

	// RemoveDevice removes a device from the gateway. The deviceID might be blank if the device isn't mapped
	// yet. If the device doesn't exist in the gatewy nil should be returned.
	RemoveDevice(localID string, deviceID string) error

	// UpdateDevice creates or updates a device on the gateway. If the device has an updated configuration
	// (f.e. updated configuration as a result of a change on the device) the updated config is returned as a
	// map of strings. If there is no updates to communicate back to Span the configuration is nil
	UpdateDevice(localID string, localDeviceID string, config map[string]string) (string, map[string]string, error)

	// DownstreamMessage sends a message to a device on the gateway
	DownstreamMessage(localID, localDeviceID, messageID string, payload []byte) error

	// UpstreamMessage sets a callback function for upstream messages
	UpstreamMessage(upstreamCb UpstreamMessageFunc)

	// Shutdown is an optional shut down call if the handler requires it.
	Shutdown()
}
