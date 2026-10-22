package handler

// 每会话客户端协议 (见 docs/PACKETS.md)。国服 (CN, StoneAge2_nogg.exe) 是基准;
// 欧版 UI (StoneAge2_ui.exe) 通过 -client ui 或 -client auto 按 C0010 版本串识别保留。
//
//	差异            CN (1.220)            UI (2.138)
//	S0100 自定义错误 result=6               result=5 (CN 的 5 = 内置实名认证提示)
//	C0A03           6 字节 u16+0,u32+2     8 字节 u16+0,u32+4
//	S0B10 单位条目   0x65, CHAR_POS @+0x4D   0x63, CHAR_POS @+0x4B
//	文本            UTF-8 (UnicodeToMbcs u82w/w2u8)   原样字节
//	S0100 错误文本   GBK (m2w, SetAcp(936))           原样字节
//
// 国服线上文本编码 (2026-10-16 真机 + UnicodeToMbcs.dll 导出名确认):
//   ord3=SetAcp, ord2=m2w (按 SetAcp 代码页), ord11=w2u8, ord12=u82w。
//   聊天/名字/NPC/宠物收发走 ord11/ord12 → UTF-8; 只有 S0100 自定义错误走 ord2 → CP936。
//   (已删的 /enc 调试, 2026-10-16): 聊天框里 UTF-8 行正常, GBK 行乱码 (与 Wine LANG 无关); 登录错误框 GBK 正常。

import (
	"strings"

	"sa2srv/internal/mbcs"
	"sa2srv/internal/server"
)

type clientKind int

const (
	clientCN clientKind = iota
	clientUI
)

func (k clientKind) String() string {
	if k == clientUI {
		return "ui"
	}
	return "cn"
}

// ClientMode: "cn" (默认) / "ui" 强制; "auto" 按 C0010 第一段版本串 (1.220=CN, 2.138=UI, 未知=CN)。
var ClientMode = "cn"

func detectClient(protoVer string) clientKind {
	switch strings.ToLower(ClientMode) {
	case "ui", "eu", "na":
		return clientUI
	case "auto":
		if strings.HasPrefix(protoVer, "2.") {
			return clientUI
		}
	}
	return clientCN
}

// 会话尚未收到 C0010 时 (例如 legacy 脚本) 按 ClientMode 决定。
func kindOf(s *session) clientKind {
	if s.kindSet {
		return s.kind
	}
	return detectClient("")
}

func connKind(c server.Conn) clientKind { return kindOf(getSess(c.RemoteAddr())) }

// charset: 一般文本字段的线上编码。两版都按原样字节 (内部 UTF-8): CN 客户端本身用 UTF-8。
func (k clientKind) charset() mbcs.Charset { return mbcs.Raw }

// loginTextCharset: S0100 自定义错误文本 (CN 0x4197dd 调 ord2 m2w, 代码页 936)。
func (k clientKind) loginTextCharset() mbcs.Charset {
	if k == clientCN {
		return mbcs.GBK
	}
	return mbcs.Raw
}

func (k clientKind) loginCustomCode() uint32 {
	if k == clientCN {
		return 6
	}
	return 5
}

// encText / decText: 内部 UTF-8 <-> 该连接线上字节。
func encText(c server.Conn, s string) []byte { return mbcs.EncodeAs(connKind(c).charset(), s) }
func decText(c server.Conn, b []byte) string { return mbcs.DecodeAs(connKind(c).charset(), b) }

// putText: 固定字段写入, 最多 max-1 字节 (保留终止 0), 不拆字符。
func putText(c server.Conn, dst []byte, max int, s string) {
	if max > len(dst) {
		max = len(dst)
	}
	if max <= 0 {
		return
	}
	copy(dst, mbcs.Fit(connKind(c).charset(), s, max-1))
}

// fitText: 编码后最多 max 字节, 不拆字符。
func fitText(c server.Conn, s string, max int) []byte { return mbcs.Fit(connKind(c).charset(), s, max) }

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// cstr: 截到第一个 0。
func cstrBytes(b []byte) []byte {
	for i, x := range b {
		if x == 0 {
			return b[:i]
		}
	}
	return b
}
