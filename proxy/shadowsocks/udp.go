package shadowsocks

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	sscore "github.com/shadowsocks/go-shadowsocks2/core"
	sssocks "github.com/shadowsocks/go-shadowsocks2/socks"

	"github.com/eycorsican/go-tun2socks/core"
	"github.com/eycorsican/go-tun2socks/proxy"
)

type udpNetAddr struct {
	network string
	addr    string
}

func (a *udpNetAddr) Network() string {
	return a.network
}

func (a *udpNetAddr) String() string {
	return a.addr
}

func SocksAddrToNetAddr(addr sssocks.Addr, network string) net.Addr {
	return &udpNetAddr{network: network, addr: addr.String()}
}

type udpHandler struct {
	sync.Mutex

	cipher     sscore.Cipher
	remoteAddr net.Addr
	conns      map[core.UDPConnection]net.PacketConn
	dnsCache   *proxy.DNSCache
	timeout    time.Duration
}

func NewUDPHandler(server, cipher, password string, timeout time.Duration) core.UDPConnectionHandler {
	ciph, err := sscore.PickCipher(cipher, []byte{}, password)
	if err != nil {
		log.Fatal(err)
	}

	remoteAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		log.Fatal(err)
	}

	return &udpHandler{
		cipher:     ciph,
		remoteAddr: remoteAddr,
		conns:      make(map[core.UDPConnection]net.PacketConn, 16),
		dnsCache:   proxy.NewDNSCache(),
		timeout:    timeout,
	}
}

func (h *udpHandler) fetchUDPInput(conn core.UDPConnection, input net.PacketConn) {
	buf := core.NewBytes(core.BufSize)

	defer func() {
		h.Close(conn)
		core.FreeBytes(buf)
	}()

	for {
		input.SetDeadline(time.Now().Add(h.timeout))
		n, _, err := input.ReadFrom(buf)
		if err != nil {
			// log.Printf("read remote failed: %v", err)
			return
		}

		targetAddr := sssocks.SplitAddr(buf[:])
		_, err = conn.WriteFrom(buf[int(len(targetAddr)):n], SocksAddrToNetAddr(targetAddr, "udp"))
		if err != nil {
			log.Printf("write local failed: %v", err)
			return
		}

		_, port, err := net.SplitHostPort(targetAddr.String())
		if err != nil {
			log.Fatal("impossible error")
		}
		if port == strconv.Itoa(proxy.COMMON_DNS_PORT) {
			h.dnsCache.Store(buf[int(len(targetAddr)):n])
			return // DNS response
		}
	}
}

func (h *udpHandler) Connect(conn core.UDPConnection, target net.Addr) error {
	pc, err := net.ListenPacket("udp", "")
	if err != nil {
		return err
	}
	pc = h.cipher.PacketConn(pc)

	h.Lock()
	h.conns[conn] = pc
	h.Unlock()
	go h.fetchUDPInput(conn, pc)
	return nil
}

func (h *udpHandler) DidReceiveTo(conn core.UDPConnection, data []byte, addr net.Addr) error {
	h.Lock()
	pc, ok1 := h.conns[conn]
	h.Unlock()

	targetAddr := sssocks.ParseAddr(addr.String())

	if ok1 {
		// Try to find a DNS answer in cache.
		_, port, err := net.SplitHostPort(targetAddr.String())
		if err != nil {
			log.Fatal("impossible error")
		}
		if port == strconv.Itoa(proxy.COMMON_DNS_PORT) {
			if answer := h.dnsCache.Query(data); answer != nil {
				var buf [1024]byte
				if dnsAnswer, err := answer.PackBuffer(buf[:]); err == nil {
					_, err = conn.WriteFrom(dnsAnswer, addr)
					if err != nil {
						return errors.New(fmt.Sprintf("cache dns answer failed: %v", err))
					}
					h.Close(conn)
					return nil
				}
			}
		}

		// Write to proxy server.
		log.Printf("proxy UDP payload for target: %v", targetAddr)
		buf := append([]byte{0, 0, 0}, targetAddr...)
		buf = append(buf, data[:]...)
		_, err = pc.WriteTo(buf[3:], h.remoteAddr)
		if err != nil {
			h.Close(conn)
			return errors.New(fmt.Sprintf("write remote failed: %v", err))
		}
		return nil
	} else {
		h.Close(conn)
		return errors.New(fmt.Sprintf("proxy connection %v->%v does not exists", conn.LocalAddr(), targetAddr))
	}
}

func (h *udpHandler) Close(conn core.UDPConnection) {
	conn.Close()

	h.Lock()
	defer h.Unlock()

	if pc, ok := h.conns[conn]; ok {
		pc.Close()
		delete(h.conns, conn)
	}
}
