package gw

import (
	"time"

	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/span/pkg/pb/gateway"
)

const keepAliveInterval = time.Second * 60

// GatewayProcess is the command processor for gateways
type GatewayProcess struct {
	Stream           gateway.UserGateway_ControlStreamClient
	Commands         CommandHandler
	stateFile        string
	upstreamRequests chan *gateway.ControlRequest
}

// NewGatewayProcess creates a new stream processor
func NewGatewayProcess(stateFile string, stream gateway.UserGateway_ControlStreamClient, commands CommandHandler) *GatewayProcess {

	return &GatewayProcess{
		Stream:    stream,
		Commands:  commands,
		stateFile: stateFile,
	}
}

// Run runs the stream processor. It will not return unless an error occurs
func (sp *GatewayProcess) Run() error {
	state, err := NewStateFromFile(sp.stateFile)
	if err != nil {
		return nil
	}
	downstreamResponses := make(chan *gateway.ControlResponse)
	upstreamRequests := make(chan *gateway.ControlRequest)
	errorCh := make(chan error)
	defer close(errorCh)
	defer sp.Commands.Shutdown()

	// Timestamp to keep track of last activity on the stream(s)
	lastMessage := time.Now()

	// Start two goroutines to handle the up- and downstream messages from Span. The upstream request
	// channel sends requests from the gateway to Span and the downstream response channel is used
	// to send messages back up to Span (messages, configuration changes and so on.)
	go func() {
		for msg := range upstreamRequests {
			err := sp.Stream.Send(msg)
			if err != nil {
				errorCh <- err
				return
			}
			lastMessage = time.Now()
		}
	}()

	go func() {
		defer close(downstreamResponses)
		for {
			msg, err := sp.Stream.Recv()
			if err != nil {
				errorCh <- err
				return
			}
			downstreamResponses <- msg
			lastMessage = time.Now()
		}
	}()

	// Send a configuration request as the first message to get the updated configuration
	upstreamRequests <- &gateway.ControlRequest{
		Msg: &gateway.ControlRequest_Config{},
	}

	sp.Commands.UpstreamMessage(func(localDeviceID string, payload []byte, metadata map[string]string) {
		deviceID := state.GetReverseMapping(localDeviceID)
		if deviceID == "" {
			lg.Error("Can't process upstream message. The local device ID %s is not known. Ignoring", localDeviceID)
			return
		}
		upstreamRequests <- &gateway.ControlRequest{
			Msg: &gateway.ControlRequest_UpstreamMessage{
				UpstreamMessage: &gateway.UpstreamMessage{
					DeviceId: deviceID,
					Payload:  payload,
					Metadata: metadata,
				},
			},
		}
	})
	for {
		select {
		case res := <-downstreamResponses:
			switch msg := res.Msg.(type) {
			case *gateway.ControlResponse_KeepaliveResponse:
				break

			case *gateway.ControlResponse_GatewayUpdate:
				state.GatewayID = msg.GatewayUpdate.GatewayId
				localID, err := sp.Commands.UpdateConfig(state.LocalID, msg.GatewayUpdate.Config)
				if err != nil {
					lg.Error("Error updating gateway configuration: %v", err)
					continue
				}
				state.LocalID = localID
				state.Save(sp.stateFile)

			case *gateway.ControlResponse_DeviceRemoved:
				if err := sp.Commands.RemoveDevice(state.LocalID, state.GetMapping(msg.DeviceRemoved.DeviceId)); err != nil {
					lg.Error("Error removing device %s: %v", msg.DeviceRemoved.DeviceId, err)
					continue
				}
				state.RemoveMapping(msg.DeviceRemoved.DeviceId)

			case *gateway.ControlResponse_DeviceUpdate:
				if state.LocalID == "" {
					lg.Error("No local ID is set for the device. Will ignore the update command")
					continue
				}
				localDeviceID, newConfig, err := sp.Commands.UpdateDevice(state.LocalID, state.GetMapping(msg.DeviceUpdate.DeviceId), msg.DeviceUpdate.Config)
				if err != nil {
					lg.Error("Error updating device %s: %v", msg.DeviceUpdate.DeviceId, err)
					continue
				}
				state.SetMapping(msg.DeviceUpdate.DeviceId, localDeviceID)
				state.Save(sp.stateFile)
				if len(newConfig) > 0 {
					upstreamRequests <- &gateway.ControlRequest{
						Msg: &gateway.ControlRequest_DeviceUpdate{
							DeviceUpdate: &gateway.DeviceUpdate{
								DeviceId: msg.DeviceUpdate.DeviceId,
								Config:   newConfig,
							},
						},
					}
				}

			case *gateway.ControlResponse_DownstreamMessage:
				if err := sp.Commands.DownstreamMessage(
					state.LocalID,
					state.GetMapping(msg.DownstreamMessage.DeviceId),
					msg.DownstreamMessage.MessageId,
					msg.DownstreamMessage.Payload); err != nil {
					lg.Error("Error sending message %s to device %s: %v", msg.DownstreamMessage.MessageId, msg.DownstreamMessage.DeviceId, err)
				}

			default:
				lg.Warning("Unknown message from server: %T", res.Msg)
			}

		case err := <-errorCh:
			close(sp.upstreamRequests)
			return err

		case <-time.After(10 * time.Second):
			// Check for timeout, send keepalive if time is > keepAliveInterval
			if time.Since(lastMessage) > keepAliveInterval {
				upstreamRequests <- &gateway.ControlRequest{
					Msg: &gateway.ControlRequest_Keepalive{},
				}
			}
		}
	}
}

// Stop closes the gateway process
func (sp *GatewayProcess) Stop() {
	close(sp.upstreamRequests)
	sp.Stream.CloseSend()
}
