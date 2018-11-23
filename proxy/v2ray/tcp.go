package v2ray

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	vcore "v2ray.com/core"
	vnet "v2ray.com/core/common/net"
	vsession "v2ray.com/core/common/session"

	"github.com/eycorsican/go-tun2socks/core"
	"github.com/eycorsican/go-tun2socks/proxy"
)

type connEntry struct {
	conn   net.Conn
	target vnet.Destination
}

type handler struct {
	sync.Mutex

	ctx   context.Context
	v     *vcore.Instance
	conns map[core.Connection]*connEntry
}

func (h *handler) fetchInput(conn core.Connection) {
	h.Lock()
	c, ok := h.conns[conn]
	h.Unlock()
	if !ok {
		return
	}

	defer func() {
		h.Close(conn)
		conn.Close()
	}()

	io.Copy(c.conn, conn)
}

func NewHandler(ctx context.Context, instance *vcore.Instance) core.ConnectionHandler {
	return &handler{
		ctx:   ctx,
		v:     instance,
		conns: make(map[core.Connection]*connEntry, 16),
	}
}

func (h *handler) Connect(conn core.Connection, target net.Addr) error {
	dest := vnet.DestinationFromAddr(target)
	sid := vsession.NewID()
	ctx := vsession.ContextWithID(h.ctx, sid)
	c, err := vcore.Dial(ctx, h.v, dest)
	if err != nil {
		return errors.New(fmt.Sprintf("dial V proxy connection failed: %v", err))
	}
	h.Lock()
	h.conns[conn] = &connEntry{
		conn:   c,
		target: dest,
	}
	h.Unlock()
	go h.fetchInput(conn)
	log.Printf("new proxy connection for target: %s:%s", target.Network(), target.String())
	return nil
}

func (h *handler) DidReceive(conn core.Connection, data []byte) error {
	h.Lock()
	c, ok := h.conns[conn]
	h.Unlock()
	if ok {
		_, err := c.conn.Write(data)
		if err != nil {
			h.Close(conn)
			return errors.New(fmt.Sprintf("write remote failed: %v", err))
		}
		return nil
	} else {
		h.Close(conn)
		return errors.New(fmt.Sprintf("proxy connection %v->%v does not exists", conn.LocalAddr(), conn.RemoteAddr()))
	}
}

func (h *handler) DidSend(conn core.Connection, len uint16) {
	// unused
}

func (h *handler) DidClose(conn core.Connection) {
	h.Close(conn)
}

func (h *handler) LocalDidClose(conn core.Connection) {
	h.Close(conn)
}

func (h *handler) Close(conn core.Connection) {
	h.Lock()
	defer h.Unlock()

	if c, found := h.conns[conn]; found {
		c.conn.Close()
	}
	delete(h.conns, conn)
}
