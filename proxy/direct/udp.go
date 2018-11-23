package direct

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/eycorsican/go-tun2socks/core"
)

type udpConnId struct {
	local  string
	remote string
}

type udpHandler struct {
	sync.Mutex

	timeout  time.Duration
	udpConns map[core.UDPConnection]net.PacketConn
}

func NewUDPHandler(timeout time.Duration) core.UDPConnectionHandler {
	return &udpHandler{
		timeout:  timeout,
		udpConns: make(map[core.UDPConnection]net.PacketConn, 8),
	}
}

func (h *udpHandler) fetchUDPInput(conn core.UDPConnection, pc net.PacketConn) {
	buf := core.NewBytes(core.BufSize)

	defer func() {
		h.Close(conn)
		core.FreeBytes(buf)
	}()

	for {
		pc.SetDeadline(time.Now().Add(h.timeout))
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			// log.Printf("failed to read UDP data from remote: %v", err)
			return
		}

		_, err = conn.WriteFrom(buf[:n], addr)
		if err != nil {
			log.Printf("failed to write UDP data to TUN")
			return
		}
	}
}

func (h *udpHandler) Connect(conn core.UDPConnection, target net.Addr) error {
	pc, err := net.ListenPacket("udp", "")
	if err != nil {
		log.Printf("failed to bind udp address")
		return err
	}
	log.Printf("udp bind local: %v", pc.LocalAddr())
	h.Lock()
	h.udpConns[conn] = pc
	h.Unlock()
	go h.fetchUDPInput(conn, pc)
	return nil
}

func (h *udpHandler) DidReceiveTo(conn core.UDPConnection, data []byte, addr net.Addr) error {
	h.Lock()
	pc, ok1 := h.udpConns[conn]
	h.Unlock()

	if ok1 {
		log.Printf("send UDP payload to: %v with local: %v", addr, pc.LocalAddr())
		_, err := pc.WriteTo(data, addr)
		if err != nil {
			log.Printf("failed to write UDP payload to remote: %v", err)
			return errors.New("failed to write UDP data")
		}
		return nil
	} else {
		return errors.New(fmt.Sprintf("proxy connection %v->%v does not exists", conn.LocalAddr(), addr))
	}
}

func (h *udpHandler) DidSend(conn core.UDPConnection, len uint16) {
	// unused
}

func (h *udpHandler) DidClose(conn core.UDPConnection) {
	// unused
}

func (h *udpHandler) LocalDidClose(conn core.UDPConnection) {
	// unused
}

func (h *udpHandler) Close(conn core.UDPConnection) {
	conn.Close()

	h.Lock()
	defer h.Unlock()

	if pc, ok := h.udpConns[conn]; ok {
		pc.Close()
		delete(h.udpConns, conn)
	}
}
