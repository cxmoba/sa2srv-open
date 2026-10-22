package handler

// 组队 (国服客户端与欧版同协议号; 欧版有日志串, 国服去掉了)。
//
//	C 0x14e0 u32 目标id               SendPartyInvate (邀请)
//	S 0x14e1 u32 邀请者id             被邀请方弹确认框 (0x1406, 显示邀请者名字/等级; 忙时客户端自动回拒绝)
//	C 0x14e1 u32 邀请者id, u32 答复   SendPartyIn (答复值见 partyAnswerYes)
//	C 0x14e5 u32 目标id               SendPartyInvateCancel
//	S 0x04e1 u32 成员id, u32 队长id   OnPartyIn: 队长id=0 → "组队失败" 提示; 否则两边对象 0x440830(成员,队长) 建关系
//	S 0x14e2 u32 被邀请者, u32 结果   给邀请者: 关等待框 0x1405; 结果 0 = 同意, 非 0 = 被拒 (真机提示 "拒绝组队申请了。")
//	S 0x04e2 u32 队长, u32 离队者     相同 = 解散 (0x5302), 否则 0x530c
//	S 0x04e0 u16 n, n×0x18 {u32 id, u16 map, s16 x, s16 y, u8, u8, u16, ...}  PARTY_POS 队伍列表 (n>=2, 长度必须 = n*0x18+2)
//	C 0x04e2 u32 id                   SendPartyOut (离队)
//	C 0x14e3 u32 id                   SendPartyRemove (队长踢人)
//	C 0x14e4 u32 id                   SendPartyLeaderChange
import (
	"log"
	"sync"

	"sa2srv/internal/server"
)

var (
	partyMu  sync.Mutex
	partyOf  = map[uint32]uint32{}   // 成员id → 队长id (队长自己也在里面)
	partyMem = map[uint32][]uint32{} // 队长id → 成员 (队长在第 0 位)
	invites  = map[uint32]uint32{}   // 被邀请者 → 邀请者
)

func entryByID(id uint32) *worldEntry {
	worldMu.Lock()
	defer worldMu.Unlock()
	for _, e := range world {
		if e.sess.char != nil && e.sess.char.ID == id {
			return e
		}
	}
	return nil
}

func mapEntries(m int) []*worldEntry {
	worldMu.Lock()
	defer worldMu.Unlock()
	var r []*worldEntry
	for _, e := range world {
		if e.sess.char != nil && e.sess.char.Map == m {
			r = append(r, e)
		}
	}
	return r
}

func partyAnswerYes(v uint32) bool { return v == uint32(knob("partyyes", 0)) }

// partyList: S 0x04e0 负载。
func partyList(leader uint32) []byte {
	mem := partyMem[leader]
	b := make([]byte, 2+len(mem)*0x18)
	put16(b, 0, uint16(len(mem)))
	for i, id := range mem {
		o := 2 + i*0x18
		put32(b, o, id)
		if e := entryByID(id); e != nil {
			copy(b[o+4:o+0xe], e.pos[4:0xe]) // map, x, y, dir, act, type (同 CHAR_POS +4..+0xd)
		}
	}
	return b
}

func (h *Handler) onPartyPacket(c server.Conn, cm uint16, p []byte) (bool, error) {
	switch cm {
	case 0x14e0, 0x14e1, 0x14e3, 0x14e4, 0x14e5, 0x04e2:
	default:
		return false, nil
	}
	s := getSess(c.RemoteAddr())
	if s.char == nil {
		return true, nil
	}
	me := s.char.ID
	log.Printf("sa2: party C 0x%04x from 0x%x % x", cm, me, p)
	var arg uint32
	if len(p) >= 4 {
		arg = le32(p)
	}
	switch cm {
	case 0x14e0: // 邀请
		t := entryByID(arg)
		if t == nil || arg == me {
			return true, sysMsg(c, 0xffff4040, "邀请对象不在。")
		}
		partyMu.Lock()
		_, busy := partyOf[arg]
		if !busy {
			invites[arg] = me
		}
		partyMu.Unlock()
		if busy {
			return true, sysMsg(c, 0xffff4040, "对方已经在队伍里了。")
		}
		pl := make([]byte, 4)
		put32(pl, 0, me)
		log.Printf("sa2: party invite 0x%x -> 0x%x", me, arg)
		return true, send(t.conn, 0x14e1, pl)
	case 0x14e5: // 取消邀请
		partyMu.Lock()
		if invites[arg] == me {
			delete(invites, arg)
		}
		partyMu.Unlock()
	case 0x14e1: // 答复
		var ans uint32
		if len(p) >= 8 {
			ans = le32(p[4:])
		}
		partyMu.Lock()
		ok := invites[me] == arg
		delete(invites, me)
		partyMu.Unlock()
		inv := entryByID(arg)
		if !ok || inv == nil {
			return true, nil
		}
		if !partyAnswerYes(ans) {
			log.Printf("sa2: party 0x%x refused 0x%x (ans %d)", me, arg, ans)
			pl := make([]byte, 8)
			put32(pl, 0, me)
			put32(pl, 4, 1)
			return true, send(inv.conn, 0x14e2, pl) // 结果非 0 = 被拒 (真机提示 "拒绝组队申请了。")
		}
		pl := make([]byte, 8)
		put32(pl, 0, me)
		_ = send(inv.conn, 0x14e2, pl) // 结果 0 = 同意 (关邀请者等待框 0x1405)
		return true, h.partyJoin(arg, me)
	case 0x04e2, 0x14e3: // 离队 / 踢人 (真机 "退出队伍" 发 C04e2 u32 0 = 自己)
		if arg == 0 {
			arg = me
		}
		h.partyLeave(arg)
	case 0x14e4:
		log.Printf("sa2: party leader change 0x%x 未实现", arg)
	}
	return true, nil
}

func (h *Handler) partyJoin(leader, member uint32) error {
	partyMu.Lock()
	if l, in := partyOf[leader]; in {
		leader = l
	} else {
		partyOf[leader] = leader
		partyMem[leader] = []uint32{leader}
	}
	if partyOf[member] == leader {
		mem := append([]uint32(nil), partyMem[leader]...)
		partyMu.Unlock()
		log.Printf("sa2: party join 0x%x already in 0x%x, members %x", member, leader, mem)
		h.sendPartyList(leader)
		return nil
	}
	partyOf[member] = leader
	partyMem[leader] = append(partyMem[leader], member)
	mem := append([]uint32(nil), partyMem[leader]...)
	partyMu.Unlock()
	le := entryByID(leader)
	if le == nil {
		return nil
	}
	log.Printf("sa2: party join 0x%x -> leader 0x%x, members %x", member, leader, mem)
	pl := make([]byte, 8)
	put32(pl, 0, member)
	put32(pl, 4, leader)
	for _, e := range mapEntries(le.sess.char.Map) {
		_ = send(e.conn, 0x04e1, pl)
	}
	h.sendPartyList(leader)
	return nil
}

func (h *Handler) sendPartyList(leader uint32) {
	partyMu.Lock()
	mem := append([]uint32(nil), partyMem[leader]...)
	partyMu.Unlock()
	if len(mem) < 2 {
		return
	}
	pl := partyList(leader)
	for _, id := range mem {
		if e := entryByID(id); e != nil {
			_ = send(e.conn, 0x04e0, pl)
		}
	}
}

// partyLeave: id 离队; 队长离队或只剩一人 → 解散。通知: S 0x04e1 {id, 0}? 未确认, 先用 0x04e2 回显。
func (h *Handler) partyLeave(id uint32) {
	partyMu.Lock()
	leader, in := partyOf[id]
	if !in {
		partyMu.Unlock()
		return
	}
	var gone []uint32
	if id == leader || len(partyMem[leader]) <= 2 {
		gone = partyMem[leader]
		for _, m := range gone {
			delete(partyOf, m)
		}
		delete(partyMem, leader)
	} else {
		delete(partyOf, id)
		var rest []uint32
		for _, m := range partyMem[leader] {
			if m != id {
				rest = append(rest, m)
			}
		}
		partyMem[leader] = rest
		gone = []uint32{id}
	}
	all := append([]uint32{leader}, gone...)
	partyMu.Unlock()
	log.Printf("sa2: party leave 0x%x (leader 0x%x), out %x", id, leader, gone)
	// S 0x04e2 {u32 队长, u32 离队者}: 两者相同 = 解散 (提示 0x5302); 否则提示 0x530c "<离队者>..."。
	out := gone
	if len(gone) > 1 { // 解散
		out = []uint32{leader}
	}
	if le := entryByID(leader); le != nil {
		for _, g := range out {
			pl := make([]byte, 8)
			put32(pl, 0, leader)
			put32(pl, 4, g)
			for _, o := range mapEntries(le.sess.char.Map) {
				_ = send(o.conn, 0x04e2, pl)
			}
		}
	}
	// 队员对象在客户端里是挂在队长身后画的; 离队后按服务器位置重发, 让他们在原地出现。
	for _, id := range all {
		if e := entryByID(id); e != nil {
			resendOthers(e.conn, e.sess)
		}
	}
	h.sendPartyList(leader)
}

// partyDrop: 断线时离队。
func (h *Handler) partyDrop(id uint32) { h.partyLeave(id) }

// playerInfo: S 0x0401 CharaInfo 给玩家对象 (id>>28 == 0): u32 id + 0xbd 字节记录 (国服 0x4bba60 整段拷进 0x7186c4 表)。
// 记录: +4 名字 (组队邀请框 0x501b90 取 +4), +0x24 u8 等级 (同), +0x31 MBCS 串 (客户端收包时转码; 用途未定)。
func playerInfo(c server.Conn, s *session) []byte {
	b := make([]byte, 4+0xbd)
	put32(b, 0, s.char.ID)
	put32(b, 4, s.char.ID)
	copy(b[4+4:4+4+0x1f], fitText(c, s.char.Name, 0x1f))
	b[4+0x24] = byte(s.level) // s.char.Level 为 0 表示未存过 (用 baseLevel), 要用会话里的实际等级
	copy(b[4+0x31:4+0x31+0x1f], fitText(c, s.char.Name, 0x1f))
	return b
}

func partyFollowers(leader uint32) []uint32 {
	partyMu.Lock()
	defer partyMu.Unlock()
	if partyOf[leader] != leader {
		return nil
	}
	return append([]uint32(nil), partyMem[leader][1:]...)
}

func inParty(id uint32) bool {
	partyMu.Lock()
	defer partyMu.Unlock()
	_, ok := partyOf[id]
	return ok
}

// partyFollow: 队长走了一步 (从 ox,oy 到现在)。队员客户端自己画 "跟在队长后面" (真机: 组队后 walkB 紧跟 Tester),
// 服务器只把队员的权威位置排成一串 (第 k 个队员站到第 k-1 个的旧位置), 存库, 再告诉队伍以外的同图玩家。
func (h *Handler) partyFollow(leader uint32, m, ox, oy int) {
	fs := partyFollowers(leader)
	if len(fs) == 0 {
		return
	}
	px, py := ox, oy
	for _, id := range fs {
		e := entryByID(id)
		if e == nil || e.sess.char.Map != m {
			continue
		}
		cx, cy := e.sess.char.X, e.sess.char.Y
		if cx == px && cy == py {
			continue
		}
		e.sess.char.X, e.sess.char.Y = px, py
		worldMu.Lock()
		put16(e.pos[:], 6, uint16(px))
		put16(e.pos[:], 8, uint16(py))
		p := e.pos
		worldMu.Unlock()
		if h.saac != nil {
			_ = h.saac.SavePos(id, m, px, py)
		}
		for _, o := range mapEntries(m) {
			if !inParty(o.sess.char.ID) {
				_ = send(o.conn, 0x0410, p[:])
			}
		}
		px, py = cx, cy
	}
}

// partyWarp: 队长换图后队员一起过去, 再在新图重发组队关系 (新图上的对象是重建的)。
func (h *Handler) partyWarp(leader uint32, m, x, y int) {
	fs := partyFollowers(leader)
	if len(fs) == 0 {
		return
	}
	for _, id := range fs {
		if e := entryByID(id); e != nil && e.sess.char.Map != m {
			h.changeMap(e.conn, e.sess, m, x, y)
		}
	}
	for _, id := range fs {
		pl := make([]byte, 8)
		put32(pl, 0, id)
		put32(pl, 4, leader)
		for _, o := range mapEntries(m) {
			_ = send(o.conn, 0x04e1, pl)
		}
	}
	h.sendPartyList(leader)
	log.Printf("sa2: party warp leader 0x%x + %d followers -> map %d (%d,%d)", leader, len(fs), m, x, y)
}

// resendOthers: 把同图其他玩家的 CHAR_POS 重发给 c (战斗回图 / 离队后)。
func resendOthers(c server.Conn, s *session) {
	if s.char == nil {
		return
	}
	for _, o := range mapEntries(s.char.Map) {
		if o.sess != s {
			worldMu.Lock()
			p := o.pos
			worldMu.Unlock()
			_ = send(c, 0x0410, p[:])
		}
	}
}

// isFollower: 在队伍里但不是队长 (队员不自己遇敌, 跟着队长进战斗)。
func isFollower(id uint32) bool {
	partyMu.Lock()
	defer partyMu.Unlock()
	l, ok := partyOf[id]
	return ok && l != id
}
