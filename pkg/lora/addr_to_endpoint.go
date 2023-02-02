package lora

import (
	"fmt"
	"net"
)

// addrToEndpoint converts a net.Addr (net.TCPAddr) into a port:host string
func addrToEndpoint(addr net.Addr) string {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		panic(fmt.Sprintf("Not a tcp address for address: %T", addr))
	}
	if tcpAddr.IP.IsUnspecified() {
		return fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port)
	}
	return fmt.Sprintf("%s:%d", tcpAddr.IP, tcpAddr.Port)
}
