package shadowsocks

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	sscore "github.com/shadowsocks/go-shadowsocks2/core"
	sssocks "github.com/shadowsocks/go-shadowsocks2/socks"

	tun2socks "github.com/eycorsican/go-tun2socks"
	"github.com/eycorsican/go-tun2socks/lwip"
	"github.com/eycorsican/go-tun2socks/proxy"
)

type udpHandler struct {
	sync.Mutex

	cipher      sscore.Cipher
	remoteAddr  net.Addr
	conns       map[tun2socks.Connection]net.PacketConn
	targetAddrs map[tun2socks.Connection]sssocks.Addr
	dnsCache    *proxy.DNSCache
	timeout     time.Duration
}

func NewUDPHandler(server, cipher, password string, timeout time.Duration) tun2socks.ConnectionHandler {
	ciph, err := sscore.PickCipher(cipher, []byte{}, password)
	if err != nil {
		log.Fatal(err)
	}

	remoteAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Fatal(err)
	}

	return &udpHandler{
		cipher:      ciph,
		remoteAddr:  remoteAddr,
		conns:       make(map[tun2socks.Connection]net.PacketConn, 16),
		targetAddrs: make(map[tun2socks.Connection]sssocks.Addr, 16),
		dnsCache:    proxy.NewDNSCache(),
		timeout:     timeout,
	}
}

func (h *udpHandler) fetchUDPInput(conn tun2socks.Connection, input net.PacketConn) {
	buf := lwip.NewBytes(lwip.BufSize)

	defer func() {
		h.Close(conn)
		lwip.FreeBytes(buf)
	}()

	for {
		input.SetDeadline(time.Now().Add(h.timeout))
		n, _, err := input.ReadFrom(buf)
		if err != nil {
			log.Printf("read remote failed: %v", err)
			return
		}

		addr := sssocks.SplitAddr(buf[:])
		_, err = conn.Write(buf[int(len(addr)):n])
		if err != nil {
			log.Printf("write local failed: %v", err)
			return
		}

		h.Lock()
		targetAddr, ok2 := h.targetAddrs[conn]
		h.Unlock()
		if ok2 {
			_, port, err := net.SplitHostPort(targetAddr.String())
			if err != nil {
				log.Fatal("impossible error")
			}
			if port == "53" {
				h.dnsCache.Store(buf[int(len(addr)):n])
			}
		}
	}
}

func (h *udpHandler) Connect(conn tun2socks.Connection, target net.Addr) error {
	pc, err := net.ListenPacket("udp", "")
	if err != nil {
		return err
	}
	pc = h.cipher.PacketConn(pc)

	h.Lock()
	h.conns[conn] = pc
	h.targetAddrs[conn] = sssocks.ParseAddr(target.String())
	h.Unlock()
	go h.fetchUDPInput(conn, pc)
	return nil
}

func (h *udpHandler) DidReceive(conn tun2socks.Connection, data []byte) error {
	h.Lock()
	pc, ok1 := h.conns[conn]
	targetAddr, ok2 := h.targetAddrs[conn]
	h.Unlock()

	if ok2 {
		_, port, err := net.SplitHostPort(targetAddr.String())
		if err != nil {
			log.Fatal("impossible error")
		}
		if port == "53" {
			if answer := h.dnsCache.Query(data); answer != nil {
				var buf [1024]byte
				if dnsAnswer, err := answer.PackBuffer(buf[:]); err == nil {
					_, err = conn.Write(dnsAnswer)
					if err != nil {
						return errors.New(fmt.Sprintf("cache dns answer failed: %v", err))
					}
					h.Close(conn)
					return nil
				}
			}
		}
	}

	if ok1 && ok2 {
		buf := append([]byte{0, 0, 0}, targetAddr...)
		buf = append(buf, data[:]...)
		_, err := pc.WriteTo(buf[3:], h.remoteAddr)
		if err != nil {
			h.Close(conn)
			return errors.New(fmt.Sprintf("write remote failed: %v", err))
		}
		return nil
	} else {
		h.Close(conn)
		return errors.New(fmt.Sprintf("proxy connection does not exists: %v <-> %v", conn.LocalAddr(), conn.RemoteAddr()))
	}
}

func (h *udpHandler) DidSend(conn tun2socks.Connection, len uint16) {
	// unused
}

func (h *udpHandler) DidClose(conn tun2socks.Connection) {
	// unused
}

func (h *udpHandler) DidAbort(conn tun2socks.Connection) {
	// unused
}

func (h *udpHandler) DidReset(conn tun2socks.Connection) {
	// unused
}

func (h *udpHandler) LocalDidClose(conn tun2socks.Connection) {
	// unused
}

func (h *udpHandler) Close(conn tun2socks.Connection) {
	conn.Close()

	h.Lock()
	defer h.Unlock()

	if pc, ok := h.conns[conn]; ok {
		pc.Close()
		delete(h.conns, conn)
	}
	delete(h.targetAddrs, conn)
}