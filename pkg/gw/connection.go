package gw

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"time"

	"github.com/lab5e/mofunk/pkg/moclient"
	"github.com/lab5e/span/pkg/pb/gateway"
	"github.com/lab5e/span/pkg/span"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Create creates a new gateway process, connects to the Span service and launches the command
// processing. The handler implements the actual gateway
func Create(config Parameters, handler CommandHandler) (*GatewayProcess, error) {
	creds, err := loadCertificates(config.CertFile, config.Chain, config.KeyFile)
	if err != nil {
		return nil, err
	}
	clusterClient, err := moclient.NewClient(&config.Cluster)
	if err != nil {
		return nil, err
	}
	if err := clusterClient.WaitForEndpoint(span.UserGatewayEndpoint, 10*time.Second); err != nil {
		return nil, err
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
		return nil, errors.New("no matching endpoints")
	}

	cc, err := grpc.Dial(
		actualEndpoint,
		grpc.WithTransportCredentials(creds))

	if err != nil {
		return nil, err
	}

	demoGW := gateway.NewUserGatewayClient(cc)
	stream, err := demoGW.ControlStream(context.Background())
	if err != nil {
		return nil, err
	}

	return NewGatewayProcess(config.StateFile, stream, handler), nil
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
