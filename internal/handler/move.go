package handler

// 移动同步。详见 PACKETS.md。
//
// 0x0410 CHAR_POS (0x18 字节, 双向, 客户端格式串 "CHAR_POS id0x%08x,x%d,y%d,area%d,dir%d,act%d,type=%d,effect_" @0x5f6500):
//   +0 u32 id (高 4 位 = 对象类型, 0x429554 shr 0x1c)  +4 u16 area(地图)  +6 s16 x  +8 s16 y
//   +0xa u8 dir  +0xb u8 act  +0xc u16 type(形象)  +0xe u16 effect  +0x10 u32  +0x14 u16  +0x16 u16
// 0x0412 = 0x0410 的增量 (SASock 层, 收: 0x57540c 解成完整 0x0410; 发: 0x574a67 压缩):
//   u8 flags, u32 id, [f&1: u16 area], f&2 ? (u16 x,u16 y) : (s8 dx, s8 dy), [f&4 u8 dir], [f&8 u8 act],
//   [f&0x10 u16 type], [f&0x20 u16 effect], [f&0x40 u32 +0x10], [f&0x80 u16 +0x14, u16 +0x16]
// 客户端自己算寻路, 每步/定时上报 (真机观察: 进图 1 次 0x0410, 之后每 60s 一次 flags=0 的 0x0412 心跳)。
// 服务器: 解码 (rx 基准) → 校验 → 存库 → 广播给同图其他人。合法移动不回自己; 只有拒绝时回 S0410 纠正。
// 别人的 0x0410: 0x429500 在 id != 自己时按 area 查地图并创建/更新角色; 0x0415 移除。

import (
	"encoding/binary"
	"log"
	"sync"

	"sa2srv/internal/cmd"
	"sa2srv/internal/proto"
	"sa2srv/internal/server"
)

type worldEntry struct {
	conn server.Conn
	sess *session
	pos  [objPosSize]byte // 权威位置: 最后一次通过规则校验的完整 CHAR_POS
	// rx: 入站流解码基准 = 客户端上一条发出的完整状态 (无论是否被规则拒绝)。
	// 客户端 TX 基准 (+0x62088) 每次发送都推进, 收到 S0410 纠正也不回退; 用 pos 解 0412 会在拒绝后错位。
	rx [objPosSize]byte
}

var (
	worldMu sync.Mutex
	world   = map[string]*worldEntry{} // key = RemoteAddr
)

func decodeDelta(base [objPosSize]byte, d []byte) ([objPosSize]byte, bool) {
	p := base
	if len(d) < 5 {
		return p, false
	}
	fl := d[0]
	copy(p[0:4], d[1:5])
	i := 5
	need := func(n int) bool { return i+n <= len(d) }
	if fl&1 != 0 {
		if !need(2) {
			return p, false
		}
		copy(p[4:6], d[i:i+2])
		i += 2
	}
	if fl&2 != 0 {
		if !need(4) {
			return p, false
		}
		copy(p[6:10], d[i:i+4])
		i += 4
	} else {
		if !need(2) {
			return p, false
		}
		x := int16(binary.LittleEndian.Uint16(p[6:])) + int16(int8(d[i]))
		y := int16(binary.LittleEndian.Uint16(p[8:])) + int16(int8(d[i+1]))
		binary.LittleEndian.PutUint16(p[6:], uint16(x))
		binary.LittleEndian.PutUint16(p[8:], uint16(y))
		i += 2
	}
	for _, f := range []struct {
		bit byte
		off int
		n   int
	}{{4, 0xa, 1}, {8, 0xb, 1}, {0x10, 0xc, 2}, {0x20, 0xe, 2}, {0x40, 0x10, 4}, {0x80, 0x14, 4}} {
		if fl&f.bit != 0 {
			if !need(f.n) {
				return p, false
			}
			copy(p[f.off:f.off+f.n], d[i:i+f.n])
			i += f.n
		}
	}
	return p, true
}

func (h *Handler) enterWorld(c server.Conn, s *session) {
	if s.char == nil {
		return
	}
	e := &worldEntry{conn: c, sess: s}
	copy(e.pos[:], objPos(s.char.ID, uint16(s.char.Map), uint16(s.char.X), uint16(s.char.Y), 4, 30002))
	e.rx = e.pos
	worldMu.Lock()
	first := true
	if old := world[c.RemoteAddr()]; old != nil && old.sess == s {
		e.rx = old.rx // 同一连接重入 (换图/回图): 客户端 TX 基准不重置
		first = false
	}
	worldMu.Unlock()
	if first {
		_ = sendOptions(c, s) // S0800 选项 (trade.go): 不发则客户端自动拒绝交易
	}
	worldMu.Lock()
	world[c.RemoteAddr()] = e
	var others []*worldEntry
	for k, o := range world {
		if k != c.RemoteAddr() && o.sess.char != nil && o.sess.char.Map == s.char.Map {
			others = append(others, o)
		}
	}
	worldMu.Unlock()
	for _, o := range others {
		_ = send(c, cmd.SV_ObjPos, o.pos[:])
		_ = send(o.conn, cmd.SV_ObjPos, e.pos[:])
	}
	log.Printf("sa2: world enter id=0x%x map=%d, %d others", s.char.ID, s.char.Map, len(others))
}

func (h *Handler) leaveWorld(c server.Conn) {
	if s := getSess(c.RemoteAddr()); s != nil && s.char != nil {
		h.partyDrop(s.char.ID) // 断线离队 (先于移出 world, 好给同图人发 0x04e2)
		h.tradeDrop(s.char.ID) // 断线取消交易, 通知对方关窗
	}
	worldMu.Lock()
	e := world[c.RemoteAddr()]
	delete(world, c.RemoteAddr())
	var others []*worldEntry
	if e != nil && e.sess.char != nil {
		for _, o := range world {
			if o.sess.char != nil && o.sess.char.Map == e.sess.char.Map {
				others = append(others, o)
			}
		}
	}
	worldMu.Unlock()
	if e == nil || e.sess.char == nil {
		return
	}
	pl := make([]byte, 4)
	put32(pl, 0, e.sess.char.ID)
	for _, o := range others {
		_ = send(o.conn, cmd.SV_CharOut, pl)
	}
}

func (h *Handler) onMoveDelta(c server.Conn, f *proto.Frame) error {
	worldMu.Lock()
	e := world[c.RemoteAddr()]
	if e == nil {
		worldMu.Unlock()
		log.Printf("sa2: 0x0412 before enter-world from %s", c.RemoteAddr())
		return nil
	}
	p, ok := decodeDelta(e.rx, f.Payload)
	if ok {
		e.rx = p // 语法有效即推进解码基准, 与规则是否接受无关
	}
	worldMu.Unlock()
	if !ok {
		log.Printf("sa2: bad 0x0412 % x", f.Payload)
		return nil
	}
	return h.applyMove(c, p, false)
}

func (h *Handler) onMove(c server.Conn, f *proto.Frame) error {
	if len(f.Payload) < objPosSize {
		log.Printf("sa2: move short %d", len(f.Payload))
		return nil
	}
	var p [objPosSize]byte
	copy(p[:], f.Payload)
	worldMu.Lock()
	if e := world[c.RemoteAddr()]; e != nil {
		e.rx = p
	}
	worldMu.Unlock()
	return h.applyMove(c, p, true)
}

// validPos: 子格坐标须在地图 gridW×gridH 内 (地图头 +0xC/+0xE)。
// 逐格可走性 (mapdata tile/chipinfo) 语义尚未逆向确认, 暂不判 (见 PACKETS.md)。
func validPos(m, x, y int) bool {
	w, hh, err := mapGrid(m)
	if err != nil {
		return x >= 0 && y >= 0
	}
	return x >= 0 && y >= 0 && x < w && y < hh
}

func (h *Handler) applyMove(c server.Conn, p [objPosSize]byte, full bool) error {
	s := getSess(c.RemoteAddr())
	worldMu.Lock()
	e := world[c.RemoteAddr()]
	worldMu.Unlock()
	if s.char == nil || e == nil {
		log.Printf("sa2: move before enter-world from %s", c.RemoteAddr())
		return nil
	}
	id := binary.LittleEndian.Uint32(p[0:])
	m := int(binary.LittleEndian.Uint16(p[4:]))
	x := int(int16(binary.LittleEndian.Uint16(p[6:])))
	y := int(int16(binary.LittleEndian.Uint16(p[8:])))
	if id != s.char.ID || m != s.char.Map || !validPos(m, x, y) {
		// 纠正: 把服务器认可的位置发回去
		log.Printf("sa2: move REJECT id=0x%x map=%d (%d,%d) -> correct to (%d,%d)", id, m, x, y, s.char.X, s.char.Y)
		return send(c, cmd.SV_ObjPos, e.pos[:])
	}
	moved := x != s.char.X || y != s.char.Y
	steps := max(abs(x-s.char.X), abs(y-s.char.Y))
	ox, oy := s.char.X, s.char.Y
	s.char.X, s.char.Y = x, y
	worldMu.Lock()
	e.pos = p
	var others []*worldEntry
	for k, o := range world {
		if k != c.RemoteAddr() && o.sess.char != nil && o.sess.char.Map == m {
			others = append(others, o)
		}
	}
	worldMu.Unlock()
	if moved && h.saac != nil {
		_ = h.saac.SavePos(id, m, x, y)
	}
	log.Printf("sa2: move id=0x%x map=%d (%d,%d) dir=%d act=%d full=%v moved=%v -> %d others", id, m, x, y, p[0xa], p[0xb], full, moved, len(others))
	for _, o := range others {
		_ = send(o.conn, cmd.SV_ObjPos, p[:])
	}
	if moved {
		h.partyFollow(id, m, ox, oy)
	}
	if moved && h.checkWarp(c, s, m, x, y) {
		return nil
	}
	if moved && stepEncounter(c, s, m, steps) {
		return nil
	}
	// 合法移动不回发给自己! 真机: 回发自己的 0x0410 = 服务器强制定位 (0x429500 id==自己 → [obj+0x1d4]),
	// 会打断客户端寻路, 每次点击只走 1 格。ServerDummy 的 C0x410 处理 (0x41c6c0) 也只在
	// 踩到传送点 (0x5ad1e0 / 0x4fd900 查到目标) 时才回 0x0410。
	return nil
}
