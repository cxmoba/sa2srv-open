package handler

// 玩家交易 (国服与欧版同协议号; 欧版日志串 SendTradeApplies / OnTradeReqResult / SendTradeItem / OnTradeOK / OnTradeResult)。
//
//	C 0x1600 u32 对方id                 申请交易 → S 0x1600 u32 申请者id 给对方 (弹确认框; 忙时客户端自动回拒绝)
//	C 0x1602 u32 对方id                 取消申请
//	C 0x1601 u32 申请者id, u32 答复     0 = 同意
//	S 0x1601 u32 对方id, u32 结果       关等待框 0x146f/0x1470; 0 → 提示 0x5468 (带名字) 并打开交易窗口 0x146e(对方); >=1 → 提示 0x5462 (被拒)
//	C 0x0690 u16 n, n×u32 id            真机: 点 "OK" 时发整份 (之后弹 "确定要这样交换吗？" 再发 C06a0)。
//	                                    id 最高位 = 石币: 0x80000000|金额 ("交换的石币" 输入框); 其余是物品 id。
//	                                    → S 0x0690 同格式给对方 (对方客户端用 C0510 u32 物品id 查物品资料)
//	C 0x06a0 u32 对方id, u32 1          最终确认 → S 0x06a0 u32 id, u32 0 给对方; 双方都确认 → 交换, S 0x06a4 {对方, 1} → "交易成功了。"
//	C 0x0680 u32 对方id, u32 0          取消 → S 0x0680 u32 id, u32 0 给对方 (关窗口, 提示 0x5467)
//	S 0x06a4 u32 对方id, u32 结果       交易结束 (关窗口; 结果 1 = 成功?)
import (
	"log"
	"sync"

	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

type tradeSide struct {
	partner uint32
	items   []uint32
	ok      bool
}

var (
	tradeMu   sync.Mutex
	trades    = map[uint32]*tradeSide{} // 交易中: 自己id → 状态
	tradeReqs = map[uint32]uint32{}     // 被申请者 → 申请者
)

func pair(a, b uint32) []byte {
	pl := make([]byte, 8)
	put32(pl, 0, a)
	put32(pl, 4, b)
	return pl
}

func (h *Handler) onTradePacket(c server.Conn, cm uint16, p []byte) (bool, error) {
	switch cm {
	case 0x1600, 0x1601, 0x1602, 0x0680, 0x0690, 0x06a0:
	default:
		return false, nil
	}
	s := getSess(c.RemoteAddr())
	if s.char == nil {
		return true, nil
	}
	me := s.char.ID
	log.Printf("sa2: trade C 0x%04x from 0x%x % x", cm, me, p)
	var a0, a1 uint32
	if len(p) >= 4 {
		a0 = le32(p)
	}
	if len(p) >= 8 {
		a1 = le32(p[4:])
	}
	switch cm {
	case 0x1600:
		t := entryByID(a0)
		if t == nil || a0 == me {
			return true, nil
		}
		tradeMu.Lock()
		tradeReqs[a0] = me
		tradeMu.Unlock()
		pl := make([]byte, 4)
		put32(pl, 0, me)
		return true, send(t.conn, 0x1600, pl)
	case 0x1602:
		tradeMu.Lock()
		if tradeReqs[a0] == me {
			delete(tradeReqs, a0)
		}
		tradeMu.Unlock()
	case 0x1601:
		tradeMu.Lock()
		ok := tradeReqs[me] == a0
		delete(tradeReqs, me)
		tradeMu.Unlock()
		ap := entryByID(a0)
		if !ok || ap == nil {
			return true, nil
		}
		if a1 != 0 {
			return true, send(ap.conn, 0x1601, pair(me, 1))
		}
		tradeMu.Lock()
		trades[me] = &tradeSide{partner: a0}
		trades[a0] = &tradeSide{partner: me}
		tradeMu.Unlock()
		log.Printf("sa2: trade open 0x%x <-> 0x%x", a0, me)
		_ = send(ap.conn, 0x1601, pair(me, 0))
		return true, send(c, 0x1601, pair(a0, 0)) // 自己这边也开窗口
	case 0x0690:
		var ids []uint32
		if len(p) >= 2 {
			n := int(le16(p))
			for i := 0; i < n && 2+4*i+4 <= len(p); i++ {
				ids = append(ids, le32(p[2+4*i:]))
			}
		}
		tradeMu.Lock()
		t := trades[me]
		if t != nil {
			t.items = ids
			t.ok = false
			if o := trades[t.partner]; o != nil {
				o.ok = false
			}
		}
		tradeMu.Unlock()
		if t == nil {
			return true, nil
		}
		if pe := entryByID(t.partner); pe != nil {
			return true, send(pe.conn, 0x0690, p)
		}
	case 0x06a0:
		tradeMu.Lock()
		t := trades[me]
		var o *tradeSide
		if t != nil {
			t.ok = true
			o = trades[t.partner]
		}
		both := t != nil && o != nil && o.ok
		tradeMu.Unlock()
		if t == nil {
			return true, nil
		}
		pe := entryByID(t.partner)
		if pe != nil {
			_ = send(pe.conn, 0x06a0, pair(me, 0))
		}
		if both && pe != nil {
			h.tradeExecute(c, s, pe.conn, pe.sess)
		}
	case 0x0680:
		tradeMu.Lock()
		t := trades[me]
		delete(trades, me)
		if t != nil {
			delete(trades, t.partner)
		}
		tradeMu.Unlock()
		if t != nil {
			if pe := entryByID(t.partner); pe != nil {
				log.Printf("sa2: trade cancel by 0x%x", me)
				return true, send(pe.conn, 0x0680, pair(me, 0))
			}
		}
	}
	return true, nil
}

// tradeExecute: 双方都确定 → 交换物品 (整格搬, 能叠的叠上), 发背包变化和 S06a4。
func (h *Handler) tradeExecute(ca server.Conn, sa *session, cb server.Conn, sb *session) {
	tradeMu.Lock()
	ta, tb := trades[sa.char.ID], trades[sb.char.ID]
	delete(trades, sa.char.ID)
	delete(trades, sb.char.ID)
	tradeMu.Unlock()
	if ta == nil || tb == nil {
		return
	}
	// 先检查空间 (粗略: 给出的格数够放收到的格数)。
	if len(sb.items)-len(tb.items)+len(ta.items) > invMax || len(sa.items)-len(ta.items)+len(tb.items) > invMax {
		_ = sysMsg(ca, 0xffff4040, "背包放不下, 交易取消。")
		_ = sysMsg(cb, 0xffff4040, "背包放不下, 交易取消。")
		_ = send(ca, 0x06a4, pair(sb.char.ID, 0))
		_ = send(cb, 0x06a4, pair(sa.char.ID, 0))
		return
	}
	goldOf := func(ids []uint32) int {
		g := 0
		for _, id := range ids {
			if id&0x80000000 != 0 {
				g += int(id & 0x7fffffff)
			}
		}
		return g
	}
	ga, gb := goldOf(ta.items), goldOf(tb.items)
	if ga > sa.gold || gb > sb.gold {
		_ = sysMsg(ca, 0xffff4040, "石币不够, 交易取消。")
		_ = sysMsg(cb, 0xffff4040, "石币不够, 交易取消。")
		_ = send(ca, 0x06a4, pair(sb.char.ID, 0))
		_ = send(cb, 0x06a4, pair(sa.char.ID, 0))
		return
	}
	moved := func(cf server.Conn, from *session, ct server.Conn, to *session, ids []uint32) int {
		n := 0
		for _, id := range ids {
			if id&0x80000000 != 0 {
				continue
			}
			_, it := findItem(from, id)
			d := (*itemDef)(nil)
			if it != nil {
				d = itemDefs[it.ItemNo]
			}
			if it == nil || it.Equipped() || d == nil {
				continue
			}
			cnt := it.Count
			_ = removeItem(cf, from, it, cnt)
			if ni := giveItem(to, d, cnt); ni != nil {
				saveItem(ni)
				_ = sendItem(ct, ni)
				n++
				log.Printf("sa2: trade item %d x%d 0x%x -> 0x%x", d.no, cnt, from.char.ID, to.char.ID)
			}
		}
		return n
	}
	na := moved(ca, sa, cb, sb, ta.items)
	nb := moved(cb, sb, ca, sa, tb.items)
	if d := gb - ga; d != 0 {
		_ = addGold(ca, sa, d)
		_ = addGold(cb, sb, -d)
		log.Printf("sa2: trade gold 0x%x %+d, 0x%x %+d", sa.char.ID, d, sb.char.ID, -d)
	}
	log.Printf("sa2: trade done 0x%x gave %d, 0x%x gave %d", sa.char.ID, na, sb.char.ID, nb)
	_ = send(ca, 0x06a4, pair(sb.char.ID, uint32(knob("traderesult", 1))))
	_ = send(cb, 0x06a4, pair(sa.char.ID, uint32(knob("traderesult", 1))))
}

// itemInfoRec: S 0x0401 给物品对象 (id>>28 == 4): 记录只有 u16 物品号 (国服 0x4bbbcb)。交易窗口显示对方物品时会查。
func itemInfoRec(id uint32) []byte {
	worldMu.Lock()
	defer worldMu.Unlock()
	for _, e := range world {
		if _, it := findItem(e.sess, id); it != nil {
			b := make([]byte, 6)
			put32(b, 0, id)
			put16(b, 4, uint16(it.ItemNo))
			return b
		}
	}
	return nil
}

// 选项 (S/C 0x0800, 0x8c 字节, 客户端存到 [0x715f00]+0x16c):
//
//	+0 u32 允许位: bit0 组队, bit1 对战, bit2 交换名片 (设置窗口三个勾), bit3 交易 (国服设置窗口里没有这个勾),
//	+6 u8 背景音乐音量, +7 u8 战斗音乐音量 (客户端默认 10/10)。
//
// 客户端点 "保存设置" 发 C0800 整份; 服务器登录时不发就全是 0 → 收到交易申请一律自动拒绝 (0x412481 test [+0x16c],8)。
// 这里: 记住客户端发来的 (按角色, 内存), 进世界时回发, 并强制打开 bit3。
var (
	optMu   sync.Mutex
	options = map[uint32][]byte{}
)

const optTrade = 8

func onOptions(c server.Conn, p []byte) error {
	s := getSess(c.RemoteAddr())
	if s.char == nil || len(p) < 0x8c {
		return nil
	}
	optMu.Lock()
	options[s.char.ID] = append([]byte(nil), p[:0x8c]...)
	optMu.Unlock()
	log.Printf("sa2: options from 0x%x flags=0x%x bgm=%d btl=%d", s.char.ID, le32(p), p[6], p[7])
	return sendOptions(c, s)
}

func sendOptions(c server.Conn, s *session) error {
	optMu.Lock()
	b := append([]byte(nil), options[s.char.ID]...)
	optMu.Unlock()
	if len(b) < 0x8c {
		b = make([]byte, 0x8c)
		put32(b, 0, uint32(knob("optflags", 0x0f)))
		b[6], b[7] = 10, 10
	}
	put32(b, 0, le32(b)|optTrade)
	return send(c, 0x0800, b)
}

// anyItem: 在线玩家里按物品 id 找 (交易窗口查对方物品资料)。
func anyItem(id uint32) *saac.Item {
	worldMu.Lock()
	defer worldMu.Unlock()
	for _, e := range world {
		if _, it := findItem(e.sess, id); it != nil {
			return it
		}
	}
	return nil
}

// tradeDrop: 断线/离开世界时取消进行中的交易和申请, 通知对方关窗。
func (h *Handler) tradeDrop(id uint32) {
	tradeMu.Lock()
	t := trades[id]
	delete(trades, id)
	if t != nil {
		delete(trades, t.partner)
	}
	delete(tradeReqs, id)
	for k, v := range tradeReqs {
		if v == id {
			delete(tradeReqs, k)
		}
	}
	tradeMu.Unlock()
	if t != nil {
		if pe := entryByID(t.partner); pe != nil {
			_ = send(pe.conn, 0x0680, pair(id, 0))
		}
	}
}
