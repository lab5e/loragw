package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/mofunk/pkg/moclient"
	"github.com/lab5e/span/pkg/pb/gateway"
	"github.com/lab5e/span/pkg/span"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type param struct {
	CertFile string `kong:"help='Client Certificate',required,file"`
	Chain    string `kong:"help='Certificate chain',required,file"`
	KeyFile  string `kong:"help='Client key file',required,file"`

	Cluster moclient.Parameters `kong:"embed,prefix='cluster-'"`
}

func loadCertificates(certFile, chainFile, keyFile string) (credentials.TransportCredentials, error) {
	certs, err := os.ReadFile(chainFile)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(certs)

	cCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cCert},
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &cCert, nil
		},
		RootCAs: certPool,
	}), nil
}

func main() {
	var config param
	kong.Parse(&config)

	creds, err := loadCertificates(config.CertFile, config.Chain, config.KeyFile)
	if err != nil {
		lg.Error("Error loading certificates: %v", err)
		return
	}
	clusterClient, err := moclient.NewClient(&config.Cluster)
	if err != nil {
		lg.Error("Error creating cluster client: %v", err)
		return
	}
	if err := clusterClient.WaitForEndpoint(span.UserGatewayEndpoint, 10*time.Second); err != nil {
		lg.Error("Could not find cluster endpoint. Exiting")
		return
	}
	// Can't use the defaults here. Must do some trickery
	actualEndpoint := ""

	for _, node := range clusterClient.DiscoveryService().List() {
		ep, ok := node.Endpoints[span.UserGatewayEndpoint]
		if ok {
			actualEndpoint = ep.Address
			break
		}
	}
	if actualEndpoint == "" {
		lg.Error("Could not find a matching endpoint")
		return
	}
	cc, err := grpc.Dial(
		actualEndpoint,
		grpc.WithTransportCredentials(creds))

	if err != nil {
		lg.Error("Error dialing server: %v", err)
		return
	}
	ctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()
	demoGW := gateway.NewUserGatewayClient(cc)
	stream, err := demoGW.ControlStream(ctx)
	if err != nil {
		lg.Error("Error opening control stream: %v", err)
		return
	}
	defer stream.CloseSend()
	sendCh := make(chan *gateway.ControlRequest)
	receiveCh := make(chan *gateway.ControlResponse)
	errorCh := make(chan error)
	go func() {
		for msg := range sendCh {
			err := stream.Send(msg)
			if err != nil {
				errorCh <- err
				return
			}
		}
	}()

	go func() {
		defer close(receiveCh)
		for {
			msg, err := stream.Recv()
			if err != nil {
				errorCh <- err
				return
			}
			receiveCh <- msg
		}
	}()

	// Send a Connect message to get the updated configuration

	sendCh <- &gateway.ControlRequest{
		Msg: &gateway.ControlRequest_Config{},
	}
	go devicePayloadSender(sendCh)
	for {
		select {
		case res := <-receiveCh:
			switch msg := res.Msg.(type) {
			case *gateway.ControlResponse_KeepaliveResponse:
				lg.Debug("Got Keepalive response")

			case *gateway.ControlResponse_GatewayUpdate:
				updateConfig(msg.GatewayUpdate.GatewayId, msg.GatewayUpdate.Config, msg.GatewayUpdate.Tags)

			case *gateway.ControlResponse_DeviceRemoved:
				lg.Debug("Got device removed")

			case *gateway.ControlResponse_DeviceUpdate:
				addDevice(msg.DeviceUpdate.DeviceId, msg.DeviceUpdate.Config, msg.DeviceUpdate.Tags)

			case *gateway.ControlResponse_DownstreamMessage:
				lg.Debug("Got downstream message")

			default:
				lg.Warning("Unknown message from server: %T", res.Msg)
			}

		case err := <-errorCh:
			close(sendCh)
			lg.Error("Got error when sending/receiving: %v", err)
			return

		case <-time.After(10 * time.Second):
			lg.Info("Sending keepalive after 10 seconds")
			sendCh <- &gateway.ControlRequest{
				Msg: &gateway.ControlRequest_Keepalive{},
			}
		}
	}
}

func updateConfig(gatewayID string, config map[string]string, tags map[string]string) {
	lg.Debug("Got gateway update message with my ID %s, config %v tags %v", gatewayID, config, tags)
	gw = gatewayConfig{
		GatewayID: gatewayID,
		Config:    config,
		Tags:      tags,
	}
}

func addDevice(deviceID string, config map[string]string, tags map[string]string) {
	lg.Debug("Got device update message with device ID %s, config %v tags %v", deviceID, config, tags)
	newDevice := deviceInfo{
		DeviceID: deviceID,
		Config:   config,
		Tags:     tags,
	}
	mutex.Lock()
	defer mutex.Unlock()
	devices[deviceID] = newDevice
}

func devicePayloadSender(sendCh chan *gateway.ControlRequest) {
	for {
		time.Sleep(1 * time.Second)

		var ids []string
		mutex.Lock()
		for k := range devices {
			ids = append(ids, k)
		}
		mutex.Unlock()

		for _, id := range ids {
			buf := make([]byte, 64)
			rand.Read(buf)
			sendCh <- &gateway.ControlRequest{
				Msg: &gateway.ControlRequest_UpstreamMessage{
					UpstreamMessage: &gateway.UpstreamMessage{
						DeviceId: id,
						Payload:  buf,
					},
				},
			}
		}
	}
}

type gatewayConfig struct {
	GatewayID string
	Config    map[string]string
	Tags      map[string]string
}

type deviceInfo struct {
	DeviceID string
	Config   map[string]string
	Tags     map[string]string
}

var devices map[string]deviceInfo = make(map[string]deviceInfo)
var gw gatewayConfig
var mutex *sync.Mutex = &sync.Mutex{}
