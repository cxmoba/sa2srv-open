// Package proto SA2 客户端协议包格式编解码。
//
// SA2 包: [u16le seq][u16le len][u16le cmd][payload]
//
//	seq  序列号 (响应回同号; 心跳自增)
//	len  整个包长度 (含 6 字节 header)
//	cmd  命令码 (uint16 LE)
//	payload 命令负载
package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

// HeaderSize 包头字节数。
const HeaderSize = 6

// MaxPayload 单包负载上限 (256KB, SASock buffer=128KB, 留余量)。
const MaxPayload = 256 << 10

var (
	ErrTooLarge = errors.New("proto: payload too large")
	ErrShortHdr = errors.New("proto: short header")
)

// Frame 单条 SA2 包。
type Frame struct {
	Seq     uint16
	Len     uint16
	Cmd     uint16
	Payload []byte
}

// Encode 序列化。
func Encode(f Frame) []byte {
	ln := HeaderSize + len(f.Payload)
	buf := make([]byte, ln)
	binary.LittleEndian.PutUint16(buf[0:2], f.Seq)
	binary.LittleEndian.PutUint16(buf[2:4], uint16(ln))
	binary.LittleEndian.PutUint16(buf[4:6], f.Cmd)
	copy(buf[6:], f.Payload)
	return buf
}

// Decode 解析一个完整包。EOF 原样返回。
func Decode(r io.Reader) (Frame, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	ln := binary.LittleEndian.Uint16(hdr[2:4])
	if ln < HeaderSize {
		return Frame{}, ErrShortHdr
	}
	if int(ln)-HeaderSize > MaxPayload {
		return Frame{}, ErrTooLarge
	}
	pl := make([]byte, int(ln)-HeaderSize)
	if _, err := io.ReadFull(r, pl); err != nil {
		return Frame{}, err
	}
	return Frame{
		Seq:     binary.LittleEndian.Uint16(hdr[0:2]),
		Len:     ln,
		Cmd:     binary.LittleEndian.Uint16(hdr[4:6]),
		Payload: pl,
	}, nil
}
