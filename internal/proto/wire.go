package proto

import (
	"encoding/binary"
	"io"
)

// 客户端真实帧格式 (2026-10-11 静态逆向 StoneAge2.exe, 详见 docs/PACKETS.md):
//
//	[u16 plainLen][u16 wireLen][u16 cmd][payload...][u16 checksum]
//
//   plainLen = 6 + len(payload) + 2           (SASock::AllocSend 0x574e2b: word[0] = n+8)
//   wireLen  = 编码后长度 (codec.encode 后 0x575296: word[2] = enc+4). 恒等 codec 时 == plainLen
//   cmd      = u16 命令码
//   checksum = 末尾 2 字节. 仅当全局 flag(0x8fe760)&2 才校验 (0x40cb10); 该 flag 代码里从未被写, 默认关闭
//
// 旧的 Encode/Decode ([seq][len][cmd][payload], 无校验尾) 保留给老测试/脚本用。

// WireHeader 客户端帧头 + 校验尾 的额外字节数。
const WireOverhead = 8

// Checksum 客户端算法 (0x40cac0): sum += int8(b[i]) + (i&3), 取低 16 位。
func Checksum(b []byte) uint16 {
	var s int32
	for i, c := range b {
		s += int32(int8(c)) + int32(i&3)
	}
	return uint16(s)
}

// EncodeWire 按客户端格式编码 (恒等 codec)。校验尾写真实 checksum (客户端默认不校验, 写了也无害)。
func EncodeWire(cmd uint16, payload []byte) []byte {
	n := WireOverhead + len(payload)
	buf := make([]byte, n)
	binary.LittleEndian.PutUint16(buf[0:2], uint16(n))
	binary.LittleEndian.PutUint16(buf[2:4], uint16(n))
	binary.LittleEndian.PutUint16(buf[4:6], cmd)
	copy(buf[6:], payload)
	binary.LittleEndian.PutUint16(buf[n-2:], Checksum(buf[:n-2]))
	return buf
}

// DecodeWire 读一个客户端格式帧. Frame.Seq 存 plainLen, Frame.Len 存 wireLen, Payload 不含校验尾。
func DecodeWire(r io.Reader) (Frame, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	wl := int(binary.LittleEndian.Uint16(hdr[2:4]))
	if wl < WireOverhead {
		return Frame{}, ErrShortHdr
	}
	rest := make([]byte, wl-HeaderSize)
	if _, err := io.ReadFull(r, rest); err != nil {
		return Frame{}, err
	}
	return Frame{
		Seq:     binary.LittleEndian.Uint16(hdr[0:2]),
		Len:     uint16(wl),
		Cmd:     binary.LittleEndian.Uint16(hdr[4:6]),
		Payload: rest[:len(rest)-2],
	}, nil
}
