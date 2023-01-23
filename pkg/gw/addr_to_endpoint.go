package gw

import (
	"fmt"
	"net"
)

// AddrToEndpoint converts a net.Addr (net.TCPAddr) into a port:host string
func AddrToEndpoint(addr net.Addr) string {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		panic("Not a tcp address for address")
	}
	if tcpAddr.IP.IsUnspecified() {
		return fmt.Sprintf("127.0.0.1:%d", tcpAddr.Port)
	}
	return fmt.Sprintf("%s:%d", tcpAddr.IP, tcpAddr.Port)
}
