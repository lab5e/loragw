package main

import (
	"context"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lab5e/l5log/pkg/lg"
	"github.com/lab5e/loragw/pkg/gw"
	"github.com/lab5e/lospan/pkg/congress"
	"github.com/lab5e/lospan/pkg/pb/lospan"
	lora "github.com/lab5e/lospan/pkg/server"
	"github.com/lab5e/mofunk/pkg/moclient"
	"github.com/lab5e/span/pkg/pb/gateway"
	"github.com/lab5e/span/pkg/span"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type param struct {
	CertFile string `kong:"help='Client Certificate',required,file"`
	Chain    string `kong:"help='Certificate chain',required,file"`
	KeyFile  string `kong:"help='Client key file',required,file"`

	LoRa    lora.Parameters     `kong:"embed,prefix='lora-'"`
	Cluster moclient.Parameters `kong:"embed,prefix='cluster-'"`
}

func main() {
	var config param
	kong.Parse(&config)

	loraServer, err := congress.NewLoRaServer(&config.LoRa)
	if err != nil {
		lg.Error("Error launching LoRa server: %v", err)
		return
	}
	loraServer.Start()
	defer loraServer.Shutdown()

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

	lc, err := grpc.Dial(gw.AddrToEndpoint(loraServer.ListenAddress()),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		lg.Error("Error dialing Congress server: %v", err)
		return
	}
	loraClient := lospan.NewLospanClient(lc)

	demoGW := gateway.NewUserGatewayClient(cc)
	stream, err := demoGW.ControlStream(context.Background())
	if err != nil {
		lg.Error("Error opening control stream: %v", err)
		return
	}
	defer stream.CloseSend()

	sp := gw.NewStreamProcessor(stream, loraClient)
	sp.Run()

}
