package core

/*
#cgo CFLAGS: -I./src/include
#include "lwip/udp.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"log"
	"net"
	"unsafe"
)

type udpConn struct {
	pcb        *C.struct_udp_pcb
	handler    UDPConnectionHandler
	localAddr  net.Addr
	localIP    C.ip_addr_t
	remoteIP   C.ip_addr_t
	remotePort C.u16_t
	localPort  C.u16_t
	closed     bool
}

func NewUDPConnection(pcb *C.struct_udp_pcb, handler UDPConnectionHandler, localIP C.ip_addr_t, localPort C.u16_t) (UDPConnection, error) {
	conn := &udpConn{
		handler:   handler,
		pcb:       pcb,
		localAddr: ParseUDPAddr(IPAddrNTOA(localIP), uint16(localPort)),
		localIP:   localIP,
		localPort: localPort,
		closed:    false,
	}
	err := handler.Connect(conn, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (conn *udpConn) RemoteAddr() net.Addr {
	log.Printf("call deprecated func RemoteAddr()")
	// deprecated
	return nil
}

func (conn *udpConn) LocalAddr() net.Addr {
	return conn.localAddr
}

func (conn *udpConn) Receive(data []byte) error {
	log.Printf("call deprecated func Receive()")
	// deprecated
	return nil
}

func (conn *udpConn) ReceiveTo(data []byte, addr net.Addr) error {
	if conn.closed {
		return errors.New("connection closed")
	}
	err := conn.handler.DidReceiveTo(conn, data, addr)
	if err != nil {
		return errors.New(fmt.Sprintf("write proxy failed: %v", err))
	}
	return nil
}

func (conn *udpConn) Write(data []byte) (int, error) {
	log.Printf("call deprecated func Write()")
	// deprecated
	return 0, nil
}

func (conn *udpConn) WriteFrom(data []byte, addr net.Addr) (int, error) {
	if conn.closed {
		return 0, errors.New("connection closed")
	}
	if conn.pcb == nil {
		return 0, errors.New("nil udp pcb")
	}
	remoteIP := addr.(*net.UDPAddr).IP.String()
	remotePort := addr.(*net.UDPAddr).Port
	cremoteIP := C.struct_ip_addr{}
	IPAddrATON(remoteIP, &cremoteIP)
	buf := C.pbuf_alloc_reference(unsafe.Pointer(&data[0]), C.u16_t(len(data)), C.PBUF_ROM)

	log.Printf("sending to local from remote: %v", ParseUDPAddr(IPAddrNTOA(cremoteIP), uint16(remotePort)))
	C.udp_sendto(conn.pcb, buf, &conn.localIP, conn.localPort, &cremoteIP, C.u16_t(remotePort))
	C.pbuf_free(buf)
	return len(data), nil
}

func (conn *udpConn) Sent(len uint16) error {
	// unused
	return nil
}

func (conn *udpConn) Close() error {
	connId := udpConnId{
		src: conn.LocalAddr().String(),
	}
	conn.closed = true
	udpConns.Delete(connId)
	return nil
}

func (conn *udpConn) Err(err error) {
	// unused
}

func (conn *udpConn) Abort() {
	// unused
}

func (conn *udpConn) LocalDidClose() error {
	// unused
	return nil
}

func (conn *udpConn) Poll() error {
	// unused
	return nil
}
