package handler

// 聊天 0x0600 (双向同布局), 详见 PACKETS.md:
//   +0x00 u32 说话者 charId   +0x04 u16 频道   +0x06 u32 颜色 ARGB (客户端按频道查表 0x60b1bc)
//   +0x0A char[0x20] 说话者名字 (MBCS)   +0x2A u16 文本长度   +0x2C 文本 MBCS + '\0'
// 频道 (2026-10-12 真机确认): 0 = All 普通 (白), 1 = Whisper 密语 (红 0xffff0000), 3 = Party 队伍 (青 0xff00ffff);
// 2 推测 Guild (黄绿 0xff9acd32, 无公会时客户端不发)。
// 密语 C->S: +0x0A 名字字段 = 收件人 (Whisper 页左侧输入框); S->C: 名字字段 = 发送者。
// 另保留服务器自定义 "/w 名字 内容"。

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"strings"

	"sa2srv/internal/cmd"
	"sa2srv/internal/proto"
	"sa2srv/internal/server"
)

const chatHdr = 0x2c

// chatPacket: name/text 是内部 UTF-8, 按接收方连接 c 的客户端编码 (CN=GBK) 写入。
// 名字区 32 字节, 最多 31 字节 + 终止 0 (客户端按 NUL 串读, 满 32 会读进长度字段)。
// +0x2A 写不含终止 0 的字节数 (CN 主聊天窗自己发包时含 0, 收包显示不读此值)。
func chatPacket(c server.Conn, id uint32, ch uint16, color uint32, name, text string) []byte {
	t := encText(c, text)
	b := make([]byte, chatHdr+len(t)+1)
	put32(b, 0, id)
	put16(b, 4, ch)
	put32(b, 6, color)
	putText(c, b[0xa:0x2a], 0x20, name)
	put16(b, 0x2a, uint16(len(t)))
	copy(b[chatHdr:], t)
	return b
}

// sysMsg: 服务端系统消息 (id 0, 频道 0)。
func sysMsg(c server.Conn, color uint32, text string) error {
	return send(c, cmd.SV_Message, chatPacket(c, 0, 0, color, "", text))
}

func (h *Handler) onChat(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	if len(f.Payload) < chatHdr || s.char == nil {
		log.Printf("sa2: chat ignored (len=%d, inworld=%v)", len(f.Payload), s.char != nil)
		return nil
	}
	p := f.Payload
	ch := binary.LittleEndian.Uint16(p[4:])
	color := binary.LittleEndian.Uint32(p[6:])
	text := p[chatHdr:]
	if i := bytes.IndexByte(text, 0); i >= 0 {
		text = text[:i]
	}
	msg := decText(c, text)
	me := s.char
	log.Printf("sa2: chat ch=%d color=%08x from 0x%x %q: %q", ch, color, me.ID, me.Name, msg)

	// GM: /pet [敌怪ID] 加宠物; /fight [敌怪ID] 开战 (省略 ID = 按当前地图遇敌表随机)
	if msg == "/pet" || strings.HasPrefix(msg, "/pet ") {
		return addPet(c, s, gmArg(msg))
	}
	if strings.HasPrefix(msg, "/warp ") { // GM: /warp 地图 x y
		var m, x, y int
		if n, _ := fmt.Sscanf(strings.TrimPrefix(msg, "/warp "), "%d %d %d", &m, &x, &y); n == 3 {
			h.changeMap(c, s, m, x, y)
		}
		return nil
	}
	if strings.HasPrefix(msg, "/battlecnt ") { // GM: 出战宠上限 (同 C 0x0465)
		n := 1
		fmt.Sscanf(msg[11:], "%d", &n)
		if n < 1 {
			n = 1
		}
		if n > 5 {
			n = 5
		}
		s.battleCnt = n
		return sysMsg(c, 0xffffff80, fmt.Sprintf("battleCnt=%d", n))
	}
	if strings.HasPrefix(msg, "/petfight ") { // GM: 开关某栏出战 (不清其它; 受 battleCnt 限制)
		slot := -1
		fmt.Sscanf(msg[10:], "%d", &slot)
		for i := range s.pets {
			if s.pets[i].Slot == slot {
				role := uint32(1)
				if !s.pets[i].Battle {
					role = 0
				}
				return setPetRole(c, s, s.pets[i].PetID(), role)
			}
		}
		return sysMsg(c, 0xffff4040, "no pet in slot")
	}
	if strings.HasPrefix(msg, "/pjoin ") { // GM: /pjoin 角色名 —— 直接把同图的这个角色拉进自己的队伍 (测试用, 跳过邀请框)
		name := strings.TrimSpace(msg[7:])
		for _, o := range mapEntries(me.Map) {
			if o.sess.char != nil && o.sess.char.Name == name && o.sess.char.ID != me.ID {
				return h.partyJoin(me.ID, o.sess.char.ID)
			}
		}
		return sysMsg(c, 0xffff4040, "找不到 "+name)
	}
	if msg == "/fight" || strings.HasPrefix(msg, "/fight ") {
		var id, n int
		fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(msg, "/fight")), "%d %d", &id, &n)
		return startFightN(c, s, id, n) // /fight [敌怪ID] [只数]
	}
	if strings.HasPrefix(msg, "/iscan ") { // 探测: /iscan 起始物品号 icon 起始偏移 步长 个数 — 每条物品资料只在一个偏移放 icon
		var base, icon, off0, step, n int
		fmt.Sscanf(msg[7:], "%d %d %d %d %d", &base, &icon, &off0, &step, &n)
		n = min(max(n, 1), 40)
		l := make([]byte, 2+0x12*n)
		put16(l, 0, uint16(n))
		for k := 0; k < n; k++ {
			o := off0 + k*step
			b := make([]byte, 4+itemRecSize)
			r := b[4:]
			put16(r, 0, uint16(base+k))
			putText(c, r[4:0x42], 0x3e, fmt.Sprintf("偏移0x%x", o))
			putText(c, r[0xc6:0x13c], 0x76, fmt.Sprintf("icon %d @0x%x", icon, o))
			if o >= 2 && o+2 <= itemRecSize {
				put16(r, o, uint16(icon))
			}
			if err := send(c, CV_ItemInfo, b); err != nil {
				return err
			}
			e := l[2+0x12*k:]
			put32(e, 0, 0x40f00000+uint32(k))
			put16(e, 4, uint16(base+k))
			put16(e, 8, 0x13)
			put16(e, 0x10, uint16(k+1))
		}
		return send(c, SV_ItemList, l)
	}
	if ok, err := itemGM(c, s, msg); ok {
		return err
	}
	if strings.HasPrefix(msg, "/give ") { // GM: /give 物品号 [数量]
		var no, n int
		n = 1
		fmt.Sscanf(msg[6:], "%d %d", &no, &n)
		d := itemDefs[no]
		if d == nil || s.char == nil {
			return sysMsg(c, 0xffff4040, "没有这个物品。")
		}
		it := giveItem(s, d, max(n, 1))
		if it == nil {
			return sysMsg(c, 0xffff4040, "背包满了。")
		}
		saveItem(it)
		return send(c, SV_ItemList, itemListPacket(it))
	}
	if strings.HasPrefix(msg, "/inv") { // 探测: /inv itemno [off val]...
		f := strings.Fields(msg[4:])
		var no int
		if len(f) > 0 {
			fmt.Sscanf(f[0], "%d", &no)
		}
		b := make([]byte, 2+0x12)
		put16(b, 0, 1)
		put32(b, 2, 0x40000001+uint32(no&0xff))
		put16(b, 6, uint16(no))
		for i := 1; i+1 < len(f); i += 2 {
			var off, v int
			fmt.Sscanf(f[i], "%d", &off)
			fmt.Sscanf(f[i+1], "%d", &v)
			if off >= 6 && off < 0x12 {
				b[2+off] = byte(v)
				if v > 255 && off+1 < 0x12 {
					b[3+off] = byte(v >> 8)
				}
			}
		}
		log.Printf("sa2: probe S04b0 % x", b)
		return send(c, 0x04b0, b)
	}
	if strings.HasPrefix(msg, "/ii ") { // 探测: /ii 物品号 icon off...  发一条 S0510 测试资料
		f := strings.Fields(msg[4:])
		var no, icon int
		fmt.Sscanf(f[0], "%d", &no)
		fmt.Sscanf(f[1], "%d", &icon)
		b := make([]byte, 4+itemRecSize)
		r := b[4:]
		put16(r, 0, uint16(no))
		putText(c, r[4:0x42], 0x3e, fmt.Sprintf("探测%d", no))
		putText(c, r[0xc6:0x13c], 0x76, msg)
		for _, x := range f[2:] {
			var o int
			fmt.Sscanf(x, "%v", &o)
			if o >= 0 && o+2 <= itemRecSize {
				put16(r, o, uint16(icon))
			}
		}
		return send(c, CV_ItemInfo, b)
	}
	if msg == "/shop" {
		return openShop(c, s)
	}
	if msg == "/merchant" { // GM: 把商人 NPC 挪到身边 (测试商店用)
		for i := range npcs {
			if npcs[i].id == merchantID {
				npcs[i].m, npcs[i].x, npcs[i].y = uint16(me.Map), uint16(me.X+2), uint16(me.Y)
			}
		}
		spawnNpcs(c, me.Map)
		return nil
	}
	if strings.HasPrefix(msg, "/param ") { // 探测: /param P D
		var pn, d int
		fmt.Sscanf(msg[7:], "%d %d", &pn, &d)
		return send(c, cmd.SV_CharParam, charParamPacket(paramDelta{s.char.ID, uint16(pn), int32(d)}))
	}
	if strings.HasPrefix(msg, "/lv ") {
		var lv int
		if _, err := fmt.Sscanf(msg[4:], "%d", &lv); err == nil {
			return setLevel(c, s, lv)
		}
	}
	// 密语 (服务器约定): /w 名字 内容
	if strings.HasPrefix(msg, "/w ") {
		parts := strings.SplitN(strings.TrimSpace(msg[3:]), " ", 2)
		if len(parts) == 2 {
			target, body := parts[0], parts[1]
			found := false
			for _, o := range worldSnapshot() {
				if o.sess.char != nil && o.sess.char.Name == target {
					_ = send(o.conn, cmd.SV_Message, chatPacket(o.conn, me.ID, ch, 0xffff80ff, me.Name, "[W] "+me.Name+": "+body))
					found = true
				}
			}
			if !found {
				return send(c, cmd.SV_Message, chatPacket(c, 0, ch, 0xffff4040, "", target+" 不在线 (not online)"))
			}
			return send(c, cmd.SV_Message, chatPacket(c, me.ID, ch, 0xffff80ff, me.Name, "[W to "+target+"] "+body))
		}
	}
	if ch == 1 { // 客户端原生密语
		nm := p[0xa:0x2a]
		if i := bytes.IndexByte(nm, 0); i >= 0 {
			nm = nm[:i]
		}
		target := decText(c, nm)
		found := false
		for _, o := range worldSnapshot() {
			if o.sess.char != nil && o.sess.char.Name == target && o.conn != c {
				_ = send(o.conn, cmd.SV_Message, chatPacket(o.conn, me.ID, ch, color, me.Name, msg))
				found = true
			}
		}
		if !found && target != me.Name {
			return send(c, cmd.SV_Message, chatPacket(c, 0, 0, 0xffff4040, "", target+" 不在线 (not online)"))
		}
		log.Printf("sa2: whisper %q -> %q delivered=%v", me.Name, target, found)
		return send(c, cmd.SV_Message, chatPacket(c, me.ID, ch, color, target, msg))
	}
	n := 0
	for _, o := range worldSnapshot() {
		if o.sess.char == nil {
			continue
		}
		// 0 普通: 同图; 队伍/公会未实现, 暂同图
		if o.sess.char.Map == me.Map {
			_ = send(o.conn, cmd.SV_Message, chatPacket(o.conn, me.ID, ch, color, me.Name, msg))
			n++
		}
	}
	log.Printf("sa2: chat relayed to %d", n)
	return nil
}

func worldSnapshot() []*worldEntry {
	worldMu.Lock()
	defer worldMu.Unlock()
	out := make([]*worldEntry, 0, len(world))
	for _, e := range world {
		out = append(out, e)
	}
	return out
}

// gmArg: "/cmd N" 的 N (没有/非法 = 0)。
func gmArg(msg string) int {
	var n int
	if i := strings.IndexByte(msg, ' '); i >= 0 {
		fmt.Sscanf(strings.TrimSpace(msg[i+1:]), "%d", &n)
	}
	return n
}
