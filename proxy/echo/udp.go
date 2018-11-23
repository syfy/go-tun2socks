package echo

import (
	"log"
	"net"

	"github.com/eycorsican/go-tun2socks/core"
)

// An echo server, do nothing but echo back data to the sender.
type udpHandler struct{}

func NewUDPHandler() core.UDPConnectionHandler {
	return &udpHandler{}
}

func (h *udpHandler) Connect(conn core.UDPConnection, target net.Addr) error {
	return nil
}

func (h *udpHandler) DidReceiveTo(conn core.UDPConnection, data []byte, addr net.Addr) error {
	log.Printf("echo target addr: %v", addr)
	// Dispatch to another goroutine, otherwise will result in deadlock.
	payload := append([]byte(nil), data...)
	go func(b []byte) {
		_, err := conn.WriteFrom(b, addr)
		if err != nil {
			log.Printf("failed to echo back data: %v", err)
		}
	}(payload)
	return nil
}

func (h *udpHandler) DidSend(conn core.UDPConnection, len uint16) {
}

func (h *udpHandler) DidClose(conn core.UDPConnection) {
}

func (h *udpHandler) LocalDidClose(conn core.UDPConnection) {
}
