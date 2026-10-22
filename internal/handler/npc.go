package handler

// 最小 NPC 对话 (2026-10-12 实验/实现), 详见 PACKETS.md "NPC dialog":
//   S 0x0410 对象位置 (id 高 4 位 = 对象类型, NPC 用 1) 生成 NPC
//   S 0x0441 NPC 信息 (0x40d570): u32 id + 0x28 字节类型 1 结构 (0x5034c0) + 名字
//   C 0x0480 SendEvent u32 id (点 NPC)   -> S 0x0620 OnDLG (0x416c00)
//   S 0x0620: +0 u32 charid(NPC) +4 u32 dlgId +8 u32 buttons +0xc u32 def +0x10 u16 disptime ... +0x34 MBCS 文本\0
//   C 0x0621 SendDlgRet: u32 dlgId, u32 button, u32 charid  -> 下一页 / S 0x0622 OnDLGEnd (u32 charid, u32 dlgId)

import (
	"encoding/binary"
	"log"

	"sa2srv/internal/server"
)

const (
	SV_NpcInfo  uint16 = 0x0441
	SV_Dlg      uint16 = 0x0620
	SV_DlgEnd   uint16 = 0x0622
	CV_Event    uint16 = 0x0480
	CV_DlgRet   uint16 = 0x0621
	CV_CharInfo uint16 = 0x0401
)

type npcDef struct {
	id      uint32
	name    string
	m, x, y uint16
	gfx     uint16
}

var npcs = []npcDef{{0x10000001, "Elder", 100, 147, 147, 30002}, {merchantID, "Merchant", 100, 200, 144, 30002}}

func findNpc(id uint32) *npcDef {
	for i := range npcs {
		if npcs[i].id == id {
			return &npcs[i]
		}
	}
	return nil
}

// npcName: 国服客户端显示中文名, 欧版英文。
var npcNameCN = map[uint32]string{0x10000001: "长老", merchantID: "商人"}

func npcName(c server.Conn, n *npcDef) string {
	if connKind(c) == clientCN {
		if s, ok := npcNameCN[n.id]; ok {
			return s
		}
	}
	return n.name
}

func npcInfo(c server.Conn, n *npcDef) []byte {
	name := encText(c, npcName(c, n))
	b := make([]byte, 4+0x28+len(name)+1)
	put32(b, 0, n.id)
	put32(b, 4, n.id)                                    // 结构 +0 id
	put32(b, 8, 1)                                       // 结构 +4 ? (猜测: NPC 种类/事件)
	copy(b[12:12+0x1f], fitText(c, npcName(c, n), 0x1f)) // 结构 +8 名字 (CNPC 0x4d1d10 读 [+0x90c]+8)
	copy(b[4+0x28:], name)
	return b
}

func spawnNpcs(c server.Conn, m int) {
	for i := range npcs {
		n := &npcs[i]
		if int(n.m) != m {
			continue
		}
		_ = send(c, 0x0410, objPos(n.id, n.m, n.x, n.y, 4, n.gfx))
		_ = send(c, SV_NpcInfo, npcInfo(c, n))
		log.Printf("sa2: npc spawn 0x%08x %q at %d,%d", n.id, n.name, n.x, n.y)
	}
}

func dlgPacket(c server.Conn, npc, dlg, buttons uint32, text string) []byte {
	t := encText(c, text)
	b := make([]byte, 0x34+len(t)+1)
	put32(b, 0, npc)
	put32(b, 4, dlg)
	put32(b, 8, buttons)
	put16(b, 0x10, 300) // disptime 秒 (x30 帧); 0 = 立即超时, 客户端马上回 button 0
	if n := findNpc(npc); n != nil {
		copy(b[0x12:0x33], fitText(c, npcName(c, n), 0x20)) // 标题 (MBCS, 0x12..0x33 含终止 0)
	}
	copy(b[0x34:], t)
	return b
}

type npcDlg struct {
	text, textCN string
	buttons      uint32
}

var npcDialog = map[uint32]npcDlg{
	1: {"Welcome to Halbol, traveller! Shall I tell you about this village?", "欢迎来到哈尔波村，旅行者！要我给你介绍一下这个村子吗？", 0x3}, // 按钮位掩码, 最多 3 个 (标签表 0x60e1bc)
	2: {"Halbol is a quiet village. This private server is a research build.", "哈尔波是个安静的村子。这个私服还在研究阶段。", 1},       // 推测 1=OK
}

func (d npcDlg) textFor(c server.Conn) string {
	if connKind(c) == clientCN && d.textCN != "" {
		return d.textCN
	}
	return d.text
}

func (h *Handler) onNpcPacket(c server.Conn, cm uint16, p []byte) (bool, error) {
	switch cm {
	case CV_CharInfo:
		if len(p) >= 4 {
			if n := findNpc(binary.LittleEndian.Uint32(p)); n != nil {
				return true, send(c, SV_NpcInfo, npcInfo(c, n))
			}
			if e := entryByID(binary.LittleEndian.Uint32(p)); e != nil {
				return true, send(c, CV_CharInfo, playerInfo(c, e.sess))
			}
			if id := binary.LittleEndian.Uint32(p); id>>28 == 4 {
				if b := itemInfoRec(id); b != nil {
					return true, send(c, CV_CharInfo, b)
				}
			}
		}
		return true, nil
	case CV_Event:
		if len(p) < 4 {
			return true, nil
		}
		id := binary.LittleEndian.Uint32(p)
		n := findNpc(id)
		log.Printf("sa2: event 0x%08x npc=%v", id, n != nil)
		if n == nil {
			return true, nil
		}
		// SendEvent (0x40d600) 置输入锁 [0x6a4e0c] 0x20 (0x42a7a0); 必须回 S 0x0480 OnEventData (0x40d660 -> 0x42a800 清锁),
		// 否则对话框/世界都不再响应鼠标键盘, 也不会再发 0x0480。
		if err := send(c, CV_Event, p[:4]); err != nil {
			return true, err
		}
		if id == merchantID {
			return true, openShop(c, getSess(c.RemoteAddr()))
		}
		getSess(c.RemoteAddr()).dlg = 1
		d := npcDialog[1]
		return true, send(c, SV_Dlg, dlgPacket(c, id, 1, d.buttons, d.textFor(c)))
	case CV_DlgRet:
		if len(p) < 12 {
			return true, nil
		}
		// 真机: 对 NPC 右键/双击 -> C 0x0621 [npcId][1][0] 以及 [npcId][1][4]
		npc, dlg, btn := binary.LittleEndian.Uint32(p), binary.LittleEndian.Uint32(p[4:]), binary.LittleEndian.Uint32(p[8:])
		s := getSess(c.RemoteAddr())
		log.Printf("sa2: dlg ret npc=0x%08x dlg=%d button=0x%x (open=%d)", npc, dlg, btn, s.dlg)
		if findNpc(npc) == nil {
			return true, nil
		}
		if btn == 0 { // 关闭/超时
			s.dlg = 0
			return true, send(c, SV_DlgEnd, dlgEnd(npc, dlg))
		}
		if s.dlg == 0 { // 没有打开的对话 (超时后客户端会再补发一条), 对话只由 C 0x0480 打开
			return true, nil
		}
		if s.dlg == 1 && btn == 1 {
			s.dlg = 2
			d := npcDialog[2]
			return true, send(c, SV_Dlg, dlgPacket(c, npc, 2, d.buttons, d.textFor(c)))
		}
		s.dlg = 0
		return true, send(c, SV_DlgEnd, dlgEnd(npc, dlg))
	}
	return false, nil
}

// S 0x0622 OnDLGEnd (0x413d40): u32 charid, u32 dlgId -> 关闭注册表 0x5708 中 +0x3c0==dlgId 的 NPC 对话窗。
func dlgEnd(npc, dlg uint32) []byte {
	b := make([]byte, 8)
	put32(b, 0, npc)
	put32(b, 4, dlg)
	return b
}
