package handler

// S 0x0420 CharParam (OnCharParam 0x40e8e0 -> 0x420aa0), 2026-10-12 逆向:
//   u16 n, 然后 n 条 12 字节: +0 u32 charId, +4 u16 apply (0 = 忽略), +6 u16 param, +8 s32 delta
// id==自己 -> player vtbl+0x228 (0x492e20) -> 函数表 0x68cd08[param] -> player vtbl[0x22c+4*param](apply, delta)
// 都是 "加 delta" (不是赋值)。player+0x942 = S 0x0230 负载起点, 所以 param 与 0x0230 偏移对应:
//   1 等级 u8 (+0x04)  21 u32 (+0x05 经验?)  2 u32 (+0x09)  3 u32 (+0x0d)  4 u16 (+0x11)  5 u16 (+0x13) ...
// 详见 PACKETS.md。

import (
	"log"

	"sa2srv/internal/cmd"
	"sa2srv/internal/server"
)

const (
	ParamLevel   = 1
	ParamExp     = 2    // +0x09 状态窗经验值
	ParamNextExp = 3    // +0x0d 状态窗 NEXT
	baseLevel    = 3    // charLoginData +0x04
	baseExp      = 4000 // charLoginData +0x05
)

type paramDelta struct {
	id    uint32
	param uint16
	delta int32
}

func charParamPacket(ds ...paramDelta) []byte {
	b := make([]byte, 2+12*len(ds))
	put16(b, 0, uint16(len(ds)))
	for i, d := range ds {
		o := 2 + 12*i
		put32(b, o, d.id)
		put16(b, o+4, 1)
		put16(b, o+6, d.param)
		put32(b, o+8, uint32(d.delta))
	}
	return b
}

// setLevel 把自己等级改成 lv (GM 指令 "/lv N")。
func setLevel(c server.Conn, s *session, lv int) error {
	if s.level == 0 {
		s.level = baseLevel
	}
	if lv < 1 || lv > 255 {
		return nil
	}
	d := int32(lv - s.level)
	dn := int32(playerNext(lv) - playerNext(s.level))
	ds := append([]paramDelta{{s.char.ID, ParamLevel, d}, {s.char.ID, ParamNextExp, dn}}, statDeltas(s.char.ID, s.level, lv)...)
	s.level = lv
	saveCharStats(s)
	log.Printf("sa2: level %q -> %d (delta %d)", s.char.Name, lv, d)
	return send(c, cmd.SV_CharParam, charParamPacket(ds...))
}
