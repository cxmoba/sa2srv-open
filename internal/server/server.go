// Package server SA2 客户端协议 server 骨架。
package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"runtime/debug"

	"sa2srv/internal/proto"
)

// Handler 处理 SA2 命令。
type Handler interface {
	Handle(c Conn, f *proto.Frame) error
	OnClose(c Conn)
}

// Conn SA2 连接接口。
type Conn interface {
	io.Reader
	io.Writer
	io.Closer
	RemoteAddr() string
}

// WrapTCPConn 包装 net.Conn 为 Conn (无 bufio, server.go 自己控制读写)。
func WrapTCPConn(c net.Conn) Conn {
	return tcpConn{c: c}
}

type tcpConn struct{ c net.Conn }

func (t tcpConn) Read(p []byte) (int, error)  { return t.c.Read(p) }
func (t tcpConn) Write(p []byte) (int, error) { return t.c.Write(p) }
func (t tcpConn) Close() error                { return t.c.Close() }
func (t tcpConn) RemoteAddr() string          { return t.c.RemoteAddr().String() }

// Legacy=true 时用旧的 [seq][len][cmd][payload] 帧 (2026-08 的 Python 测试脚本格式);
// 默认 false = 客户端真实帧 [plainLen][wireLen][cmd][payload][cksum] (见 proto/wire.go)。
var Legacy bool

// Send 发 SA2 包。
func Send(c Conn, f *proto.Frame) error {
	var b []byte
	if Legacy {
		b = proto.Encode(*f)
	} else {
		b = proto.EncodeWire(f.Cmd, f.Payload)
	}
	log.Printf("SEND %s cmd=0x%04x len=%d", c.RemoteAddr(), f.Cmd, len(f.Payload))
	_, err := c.Write(b)
	return err
}

func decode(c Conn) (proto.Frame, error) {
	if Legacy {
		return proto.Decode(c)
	}
	return proto.DecodeWire(c)
}

// ServeTCP accept loop, 每连接一个 goroutine。
func ServeTCP(addr string, h Handler) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("sa2srv: listening on %s", addr)
	for {
		c, err := l.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		go serve(WrapTCPConn(c), h)
	}
}

func serve(c Conn, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("sa2srv: panic: %v (%s)\n%s", r, c.RemoteAddr(), debug.Stack())
		}
		c.Close()
		h.OnClose(c)
	}()
	log.Printf("ACCEPT %s", c.RemoteAddr())
	for {
		f, err := decode(c)
		if err != nil {
			log.Printf("decode end %s: %v", c.RemoteAddr(), err)
			return
		}
		if f.Cmd == 0x0100 { // 登录包含账号密码, 不记原文
			log.Printf("RECV %s cmd=0x%04x plain=%d wire=%d payload=<login %d bytes redacted>", c.RemoteAddr(), f.Cmd, f.Seq, f.Len, len(f.Payload))
		} else {
			log.Printf("RECV %s cmd=0x%04x plain=%d wire=%d payload=% x", c.RemoteAddr(), f.Cmd, f.Seq, f.Len, f.Payload)
		}
		if err := h.Handle(c, &f); err != nil {
			log.Printf("sa2srv: handle err: %v (%s)", err, c.RemoteAddr())
			return
		}
	}
}
