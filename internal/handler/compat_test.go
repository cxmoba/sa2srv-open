package handler

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"sa2srv/internal/proto"
	"sa2srv/internal/saac"
)

// fakeConn 记录服务端发出的帧。
type fakeConn struct {
	addr string
	out  bytes.Buffer
}

func (f *fakeConn) Read(p []byte) (int, error)  { return 0, io.EOF }
func (f *fakeConn) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f *fakeConn) Close() error                { return nil }
func (f *fakeConn) RemoteAddr() string          { return f.addr }

func (f *fakeConn) frames(t *testing.T) []proto.Frame {
	var fs []proto.Frame
	r := bytes.NewReader(f.out.Bytes())
	for r.Len() > 0 {
		fr, err := proto.DecodeWire(r)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		fs = append(fs, fr)
	}
	f.out.Reset()
	return fs
}

func newConn(addr string, k clientKind) *fakeConn {
	dropSess(addr)
	s := getSess(addr)
	s.kind, s.kindSet = k, true
	return &fakeConn{addr: addr}
}

func TestDetectClient(t *testing.T) {
	old := ClientMode
	defer func() { ClientMode = old }()
	ClientMode = "auto"
	if detectClient("2.138") != clientUI || detectClient("1.220") != clientCN || detectClient("") != clientCN {
		t.Fatal("auto detect")
	}
	ClientMode = "cn"
	if detectClient("2.138") != clientCN {
		t.Fatal("forced cn")
	}
	ClientMode = "ui"
	if detectClient("1.220") != clientUI {
		t.Fatal("forced ui")
	}
}

func TestLoginCustomCode(t *testing.T) {
	cn := newConn("t-login-cn", clientCN)
	ui := newConn("t-login-ui", clientUI)
	b := loginCustom(cn, "密码")
	if binary.LittleEndian.Uint32(b) != 6 || binary.LittleEndian.Uint16(b[4:]) != 4 || !bytes.Equal(b[6:], []byte{0xc3, 0xdc, 0xc2, 0xeb}) {
		t.Fatalf("cn % x", b)
	}
	b = loginCustom(ui, "x")
	if binary.LittleEndian.Uint32(b) != 5 {
		t.Fatalf("ui % x", b)
	}
}

func TestCharListNameOffset(t *testing.T) {
	st, err := saac.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Create("u", "p"); err != nil {
		t.Fatal(err)
	}
	ch := &saac.Char{Account: "u", Slot: 0, Name: "中文名", Map: 100, X: 1, Y: 1}
	if err := st.CreateChar(ch); err != nil {
		t.Fatal(err)
	}
	h := New(st)
	c := newConn("t-charlist", clientCN)
	getSess(c.addr).account = "u"
	if err := h.onCharList(c, &proto.Frame{}); err != nil {
		t.Fatal(err)
	}
	fs := c.frames(t)
	if len(fs) != 1 || fs[0].Cmd != 0x0220 {
		t.Fatalf("frames %v", fs)
	}
	pl := fs[0].Payload
	if binary.LittleEndian.Uint16(pl) != 1 || len(pl) != 2+2*charEntrySize {
		t.Fatalf("count/len")
	}
	e := pl[2:]
	if binary.LittleEndian.Uint32(e[2:]) != ch.ID {
		t.Fatalf("id")
	}
	want := append([]byte("中文名"), 0) // CN 名字线上为 UTF-8
	if !bytes.Equal(e[0x34:0x34+len(want)], want) {
		t.Fatalf("name at +0x34: % x", e[0x30:0x40])
	}
}

func TestBattleInitLayouts(t *testing.T) {
	us := []bUnit{
		{0x100, 0, 0, 150, 200, 10, 20, 7, 31002, true, "Tester", 4},
		{monsterID, 1, 1, 18, 18, 11, 9, 3, 27010, false, "克洛尔", 3},
	}
	for _, k := range []clientKind{clientCN, clientUI} {
		L := layoutFor(k)
		b := battleInit(k, 12000, 18, 18, us)
		if len(b) != 0xc+2*L.stride || b[0xb] != 2 {
			t.Fatalf("%s len %d", k, len(b))
		}
		for i, u := range us {
			e := b[0xc+i*L.stride:]
			chk := func(name string, got, want uint32) {
				if got != want {
					t.Fatalf("%s unit %d %s = %d want %d", k, i, name, got, want)
				}
			}
			chk("id", binary.LittleEndian.Uint32(e), u.id)
			chk("hp", binary.LittleEndian.Uint32(e[0x17:]), u.hp)
			chk("owner", binary.LittleEndian.Uint32(e[L.pos:]), u.id)
			chk("map", uint32(binary.LittleEndian.Uint16(e[L.pos+4:])), 12000)
			chk("x", uint32(binary.LittleEndian.Uint16(e[L.pos+6:])), uint32(u.x))
			chk("y", uint32(binary.LittleEndian.Uint16(e[L.pos+8:])), uint32(u.y))
			chk("dir", uint32(e[L.pos+0xa]), uint32(u.dir))
			chk("gfx", uint32(binary.LittleEndian.Uint16(e[L.pos+0xc:])), uint32(u.gfx))
			chk("lv", uint32(e[0x1c]), uint32(u.lv))
			if want := append([]byte(u.name), 0); !bytes.Equal(e[0x27:0x27+len(want)], want) {
				t.Fatalf("%s unit %d name % x", k, i, e[0x27:0x47])
			}
		}
	}
	// CN 具体偏移: map +0x51, x +0x53, y +0x55, dir +0x57, gfx +0x59; 新增 +0x4b..+0x4c 为 0
	b := battleInit(clientCN, 12000, 18, 18, us)
	e := b[0xc+0x65:]
	if binary.LittleEndian.Uint16(e[0x51:]) != 12000 || binary.LittleEndian.Uint16(e[0x53:]) != 11 ||
		binary.LittleEndian.Uint16(e[0x55:]) != 9 || e[0x57] != 3 || binary.LittleEndian.Uint16(e[0x59:]) != 27010 ||
		e[0x4b] != 0 || e[0x4c] != 0 {
		t.Fatalf("cn unit 2 tail % x", e[0x47:0x5b])
	}
	// UI 旧偏移保持
	b = battleInit(clientUI, 12000, 18, 18, us)
	e = b[0xc+0x63:]
	if binary.LittleEndian.Uint16(e[0x4f:]) != 12000 || binary.LittleEndian.Uint16(e[0x51:]) != 11 || e[0x55] != 3 || binary.LittleEndian.Uint16(e[0x57:]) != 27010 {
		t.Fatalf("ui unit 2 tail % x", e[0x47:0x59])
	}
}

func TestRewardExpZeroNotReset(t *testing.T) {
	c := newConn("t-exp", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T"}
	s.level, s.exp, s.expInit = baseLevel, baseExp, true
	rewardWin(c, s) // 5000
	rewardWin(c, s) // 6000 -> lv4, 0
	if s.level != 4 || s.exp != 0 {
		t.Fatalf("after 2 wins lv %d exp %d", s.level, s.exp)
	}
	rewardWin(c, s)
	if s.exp != winExpDef {
		t.Fatalf("exp reset: %d", s.exp)
	}
}

// 只捉不打: 被捉的怪不给经验/掉落 (之前全捉时落到固定值 SA2_WINEXP=1000)。
func TestRewardCapturedNoExp(t *testing.T) {
	c := newConn("t-capexp", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T"}
	s.level, s.exp, s.expInit = baseLevel, baseExp, true
	s.foes = []*foe{{id: monsterID, bid: bidMonster, captured: true}}
	if pl, pet := winExps(s); pl != 0 || pet != 0 {
		t.Fatalf("captured-only exp %d/%d, want 0/0", pl, pet)
	}
	rewardWin(c, s)
	if s.exp != baseExp {
		t.Fatalf("exp changed %d -> %d", baseExp, s.exp)
	}
	// 一只打死一只被捉: 只算打死的那只 (无数据表 → 固定值)
	s.foes = []*foe{{id: monsterID, bid: bidMonster, captured: true}, {id: monsterID + 1, bid: bidMonster | 0x100}}
	if pl, _ := winExps(s); pl != winExpDef {
		t.Fatalf("one killed exp %d, want %d", pl, winExpDef)
	}
}

func TestSyncTokenCheck(t *testing.T) {
	h := &Handler{}
	c := newConn("t-sync", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T"}
	s.inBattle, s.monHP, s.hp = true, 1000, 100
	s.foes = []*foe{{id: monsterID, bid: bidMonster, hp: 1000, maxHP: 1000}}
	if err := sendSync(c, s); err != nil {
		t.Fatal(err)
	}
	tok := s.syncTok
	c.frames(t)
	bad := make([]byte, 4)
	put32(bad, 0, tok+1)
	h.onBattlePacket(c, 0x0bd0, bad)
	if s.syncN != 0 || len(c.frames(t)) != 0 {
		t.Fatal("wrong token advanced")
	}
	good := make([]byte, 4)
	put32(good, 0, tok)
	h.onBattlePacket(c, 0x0bd0, good)
	if s.syncN != 1 {
		t.Fatalf("good token phase %d", s.syncN)
	}
	n := s.syncN
	h.onBattlePacket(c, 0x0bd0, good) // 重复旧 token
	if s.syncN != n {
		t.Fatal("duplicate advanced")
	}
}

func TestMoveRejectThenDelta(t *testing.T) {
	h := &Handler{}
	c := newConn("t-move", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T", Map: 100, X: 5, Y: 5}
	h.enterWorld(c, s)
	defer h.leaveWorld(c)
	// 客户端上报 x=-1 (被拒), 然后 dx=+1 → 客户端实际位置 x=0
	full := objPos(0x100, 100, 0xffff, 5, 4, 30002)
	h.onMove(c, &proto.Frame{Payload: full})
	if s.char.X != 5 {
		t.Fatalf("reject failed x=%d", s.char.X)
	}
	h.onMoveDelta(c, &proto.Frame{Payload: []byte{0, 0, 1, 0, 0, 1, 0}})
	if s.char.X != 0 || s.char.Y != 5 {
		t.Fatalf("delta after reject decoded to (%d,%d), want (0,5)", s.char.X, s.char.Y)
	}
}

func TestBattleEndOn0412(t *testing.T) {
	h := &Handler{}
	c := newConn("t-0412", clientCN)
	s := getSess(c.addr)
	s.char = &saac.Char{ID: 0x100, Name: "T", Map: 100, X: 5, Y: 5}
	s.inBattle, s.ended = true, true
	h.onBattlePacket(c, 0x0412, []byte{0, 0, 1, 0, 0, 0, 0})
	if s.inBattle || s.ended {
		t.Fatal("0412 did not clear battle state")
	}
}
