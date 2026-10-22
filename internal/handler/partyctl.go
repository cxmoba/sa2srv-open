package handler

// 组队战斗 (队员自己操作, 2026-10-21): 队长遇敌时, 同图队员的客户端也进战斗画面, 各自给自己的角色/宠物下指令。
//
//   - 战斗状态仍在队长 session 里 (s.foes / s.queue / s.allies); 队员 session.battleHost 指向队长。
//   - 开战: 每个队员收到自己视角的 S0B10 —— 单位表和队长的一样, 只有 +7 "可下指令" 换成自己的角色/宠物
//     (国服 BID 高字节和 +7 重叠: 自己的单位动作键 = BID|0x01000000, 别人的 = 裸 BID)。
//   - 队长发的 S0B30 / S0BD0 / S0BD1 (send 钩子 mirrorBattle) 抄送给每个队员, S0B30 里的行动者/目标键按队员视角改写。
//   - 指令: 队员 C0B20 记进队长的 orderAct/orderTgt (键 = 角色/宠物 id, 全服唯一), 回显给队员; 队员 C0B90 = 本回合指令下完。
//     队长计时结束 (C0BD0 phase 0) 时, 若还有活着的队员没下完 → 挂起, 等最后一个队员的 C0B90 或超时 (knob partywait 秒, 默认 35)。
//   - 结算: 队长 endBattle 之后, 每个队员用自己的 session 走同一个 endBattle (同样紧跟最后一条 S0B30 发 S0BE0),
//     经验按 SA1 _SHARE_EXP: 我方每个活着的单位各拿全额 (不平分); 掉落每只被打死的怪掷一次, 随机给一个活着的角色。
//   - 逃跑 (SA1 BATTLE_Escape): 逃跑的人自己离开战斗 (其余人继续), 战后离队。队长逃跑成功 = 全队一起退出 (简化)。
//   - knob partyctl=0: 回到旧行为 (只有队长进战斗, 队员单位由 AI 控制)。

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"sa2srv/internal/proto"
	"sa2srv/internal/server"
)

var battleMu sync.Mutex
var theHandler *Handler

type bMember struct {
	s       *session
	c       server.Conn
	charBID uint32
	ready   bool
	gone    bool // 逃跑离开
}

func (m *bMember) own(bid uint32) bool {
	if bid == m.charBID {
		return true
	}
	if bid < m.charBID+0x100 {
		return false
	}
	d := bid - (m.charBID + 0x100)
	return d%0x20 == 0 && d/0x20 < 5
}

// memberUnitsAlive: 队员的角色或宠物还有活着的。
func memberUnitsAlive(h *session, m *bMember) bool {
	for _, a := range h.allies {
		if a.owner == m.s && a.alive() {
			return true
		}
	}
	return false
}

func memberOf(h *session, s *session) *bMember {
	for _, m := range h.members {
		if m.s == s {
			return m
		}
	}
	return nil
}

func partyReady(h *session) bool {
	for _, m := range h.members {
		if !m.gone && !m.ready && memberUnitsAlive(h, m) {
			return false
		}
	}
	return true
}

// startPartyMembers: 队长 S0B10 发出后, 给每个队员发它自己视角的 S0B10。
func startPartyMembers(c server.Conn, s *session, bgID uint16, cx, cy int16, us []bUnit) {
	s.members, s.bconn, s.turnPending = nil, c, false
	if knob("partyctl", 1) == 0 {
		return
	}
	for _, a := range s.allies {
		if a.pet != nil {
			continue
		}
		e := entryByID(a.id)
		if e == nil {
			continue
		}
		ms := a.owner
		m := &bMember{s: ms, c: e.conn, charBID: a.bid}
		s.members = append(s.members, m)
		ms.battleHost = s
		ms.inBattle, ms.ended, ms.rewarded, ms.escaped, ms.syncWait = true, false, false, false, false
		ms.syncN, ms.syncSent, ms.queue, ms.members = 0, false, nil, nil
		for _, bp := range battlePets(ms) {
			_ = sendPetInfo(m.c, bp)
			_ = sendPetBattleFlag(m.c, bp.PetID())
		}
		view := make([]bUnit, len(us))
		copy(view, us)
		for i := range view {
			if view[i].side != 0 {
				continue
			}
			view[i].orderable = m.own(view[i].bid)
		}
		k := connKind(m.c)
		b := battleInit(k, bgID, cx, cy, view)
		log.Printf("sa2: party battle: member %q (0x%x) joins, char bid 0x%x, %d units", ms.char.Name, ms.char.ID, m.charBID, b[0xb])
		_ = send(m.c, SV_BattleInit, b)
	}
}

// remapKeys: S0B30 (btlflag 0) 行动者/目标键从队长视角改成 own 视角。
func remapKeys(pl []byte, own func(uint32) bool) []byte {
	b := append([]byte(nil), pl...)
	if len(b) < 2 || b[0] != 0 {
		return b
	}
	n := int(b[1])
	for i := 0; i < n; i++ {
		e := 2 + 0x4b*i
		if e+0x4b > len(b) {
			break
		}
		for _, off := range []int{4, 0x36} {
			k := le32(b[e+off:])
			bare := k
			if k>>24 == 1 {
				bare = k & 0xffffff
			}
			if own(bare) {
				bare |= 0x01000000
			}
			put32(b[e+off:], 0, bare)
		}
	}
	return b
}

// mirrorBattle: send 钩子。队长的战斗包抄送队员。
func mirrorBattle(c server.Conn, command uint16, pl []byte) {
	if command != 0x0b30 && command != 0x0bd0 && command != 0x0bd1 {
		return
	}
	s := getSess(c.RemoteAddr())
	if len(s.members) == 0 || !s.inBattle {
		return
	}
	for _, m := range s.members {
		if m.gone {
			continue
		}
		p := pl
		switch command {
		case 0x0b30:
			p = remapKeys(pl, m.own)
		case 0x0bd1:
			m.ready = false
		}
		_ = server.Send(m.c, &proto.Frame{Cmd: command, Payload: p})
	}
}

// onMemberBattle: 队员客户端的战斗包。返回 handled。
func onMemberBattle(c server.Conn, s *session, cm uint16, p []byte) (bool, error) {
	h := s.battleHost
	if h == nil || !s.inBattle || s.ended {
		return false, nil
	}
	m := memberOf(h, s)
	if m == nil || m.gone {
		return false, nil
	}
	switch cm {
	case 0x0b20:
		if len(p) >= 0x14 {
			if h.orderTgt == nil {
				h.orderTgt = map[uint32]uint32{}
			}
			if h.orderAct == nil {
				h.orderAct = map[uint32]byte{}
			}
			h.orderTgt[le32(p)] = le32(p[0x10:])
			h.orderAct[le32(p)] = p[8]
		}
		log.Printf("sa2: party battle: member %q order actor=0x%x act=0x%02x tgt=0x%x", s.char.Name, le32(p), p[8], le32(p[0x10:]))
		return true, send(c, 0x0b20, p)
	case 0x0b90:
		m.ready = true
		log.Printf("sa2: party battle: member %q order enter (pending=%v ready=%v)", s.char.Name, h.turnPending, partyReady(h))
		if h.turnPending && partyReady(h) {
			return true, runPendingTurn(h)
		}
		return true, nil
	case 0x0bd0:
		return true, nil // 节奏由队长客户端的 C0BD0 驱动
	case 0x0b01:
		if knob("memb01", 0) != 0 {
			return true, send(c, 0x0bd1, orderReq())
		}
		return true, nil
	case 0x0b02, 0x0b03, 0x0b40, 0x0b50, 0x0b60, 0x0b70, 0x0b80, 0x0bc0, 0x0be0:
		log.Printf("sa2: party battle: member C 0x%04x % x", cm, p)
		return true, nil
	}
	return false, nil
}

// holdTurn: 队长已就绪但还有队员没下完 → 挂起, 超时强制开打。返回 true = 已挂起。
// 已在等时不再重开计时 (否则每次队长 C0B90/C0BD0 都把 partywait 重置, 永远开不了打)。
func holdTurn(s *session) bool {
	if len(s.members) == 0 || partyReady(s) {
		return false
	}
	if s.turnPending {
		return true
	}
	s.turnPending = true
	s.turnGen++
	gen := s.turnGen
	d := time.Duration(knob("partywait", 35)) * time.Second
	log.Printf("sa2: party battle: waiting for members' orders (timeout %v)", d)
	time.AfterFunc(d, func() {
		battleMu.Lock()
		defer battleMu.Unlock()
		if s.turnPending && s.turnGen == gen && s.inBattle && !s.ended {
			log.Printf("sa2: party battle: order timeout, members not ready act on AI")
			_ = runPendingTurn(s)
		}
	})
	return true
}

// armTurnStart: 队长 C0B90 且全员就绪后, 若客户端没在短时间内回 C0BD0 (早到的 S0BD0 被丢), 就强制开打。
// knob turnarm 毫秒 (默认 800)。C0BD0 / runPendingTurn 会 bump turnGen 取消。
func armTurnStart(c server.Conn, s *session) {
	s.turnGen++
	gen := s.turnGen
	d := time.Duration(knob("turnarm", 800)) * time.Millisecond
	log.Printf("sa2: arm turn start in %v", d)
	time.AfterFunc(d, func() {
		battleMu.Lock()
		defer battleMu.Unlock()
		if s.turnGen != gen || !s.inBattle || s.ended || s.syncN != 0 || len(s.queue) > 0 {
			return
		}
		if holdTurn(s) {
			return
		}
		log.Printf("sa2: turn start after order-enter (no C0BD0 within %v)", d)
		buildQueue(s)
		_ = stepTurn(c, s)
	})
}

func runPendingTurn(s *session) error {
	s.turnPending = false
	s.turnGen++ // 取消 armTurnStart / 旧 partywait
	if s.bconn == nil {
		return nil
	}
	buildQueue(s)
	return stepTurn(s.bconn, s)
}

// allyOrderStep: 队员单位按自己下的指令行动。返回 handled=true 时 (sent, err) 有效; false = 照旧 AI 攻击。
func allyOrderStep(c server.Conn, s *session, a *ally) (handled, sent bool, err error) {
	act, ok := s.orderAct[a.id]
	if !ok {
		return false, false, nil
	}
	switch act {
	case actGuard:
		sent, err = guardStep(c, s, a.bid)
		return true, sent, err
	case actWait:
		log.Printf("sa2: ally %q waits", a.name)
		return true, false, nil
	case actEscape:
		if a.pet != nil {
			return true, false, nil
		}
		sent, err = memberEscape(c, s, a)
		return true, sent, err
	case actAttack, actSkill:
		return false, false, nil
	}
	log.Printf("sa2: ally %q order act=0x%02x 未实现 (队员) → 跳过", a.name, act)
	return true, false, nil
}

// memberEscape: 队员逃跑 (SA1: 逃跑者自己退出战斗并离队, 其余人继续)。
func memberEscape(c server.Conn, s *session, a *ally) (bool, error) {
	m := memberOf(s, a.owner)
	if m == nil {
		return false, nil
	}
	_, _, agi := a.stats()
	sum, n := 0, 0
	for _, f := range s.foes {
		if f.alive() {
			_, _, g := f.stats()
			sum += g
			n++
		}
	}
	avg := 0
	if n > 0 {
		avg = sum / n
	}
	pct := min(max(60+(agi-avg)*2, 10), 95)
	if k := knob("escpct", -1); k >= 0 {
		pct = k
	}
	okEsc := rand.Intn(100) < pct
	log.Printf("sa2: member %q tries to escape (%d%%) -> %v", a.name, pct, okEsc)
	act := bAct{actor: a.bid, target: a.bid, act: actEscape, setCnd: true}
	if !okEsc {
		act.efe = uint16(knob("escfail", 0x97))
	}
	if err := send(c, 0x0b30, battleAction(act)); err != nil {
		return true, err
	}
	if okEsc {
		ms := a.owner
		finishMember(s, m, true)
		_ = sysMsg(m.c, 0xffff8040, "逃跑成功!")
		s.leaveAfter = append(s.leaveAfter, ms.char.ID)
		if sideDown(s) {
			s.syncN, s.syncSent, s.queue = 0, false, nil
			endBattle(c, s)
			return true, nil
		}
	}
	s.syncN++
	return true, sendSync(c, s)
}

// finishMember: 给一个队员发结算 (走它自己 session 的 endBattle), 写回 HP; escaped = 它自己逃跑。
func finishMember(h *session, m *bMember, escaped bool) {
	ms := m.s
	if m.gone || ms.ended {
		m.gone = true
		return
	}
	ms.hasPet, ms.petHP = false, 0
	ms.petHPs = map[uint32]int{}
	ms.hp = 1
	for _, a := range h.allies {
		if a.owner != ms {
			continue
		}
		if a.pet == nil {
			ms.hp = a.hp
		} else {
			ms.hasPet = true
			ms.petHPs[a.pet.PetID()] = a.hp
			if ms.petHP == 0 {
				ms.petHP = a.hp
			}
		}
		if escaped {
			a.gone = true
		}
	}
	ms.foes, ms.mon, ms.escaped = h.foes, h.mon, escaped || h.escaped
	ms.monHP = h.monHP
	if escaped {
		ms.monHP = 1
	}
	ms.noDrops = true
	ms.battleHost = nil
	endBattle(m.c, ms)
	m.gone = true
}

// partyEnd: 队长 endBattle 之后: 其余队员结算 + 掉落分配; 逃跑的队员离队。
func partyEnd(c server.Conn, s *session, win bool) {
	for _, m := range s.members {
		finishMember(s, m, false)
	}
	if win {
		type rc struct {
			c server.Conn
			s *session
		}
		var alive []rc
		if s.hp > 0 {
			alive = append(alive, rc{c, s})
		}
		for _, m := range s.members {
			for _, a := range s.allies {
				if a.owner == m.s && a.pet == nil && a.alive() {
					alive = append(alive, rc{m.c, m.s})
				}
			}
		}
		if len(alive) == 0 {
			alive = append(alive, rc{c, s})
		}
		for _, f := range s.foes {
			if f.captured {
				continue
			}
			r := alive[rand.Intn(len(alive))]
			rollDrops(r.c, r.s, 1)
		}
	}
	s.members = nil
	if ids := s.leaveAfter; len(ids) > 0 && theHandler != nil {
		s.leaveAfter = nil
		go func() {
			time.Sleep(6 * time.Second) // 等逃跑者回到场景
			for _, id := range ids {
				theHandler.partyLeave(id)
			}
		}()
	}
}
