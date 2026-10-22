package handler

// 登录 -> 选角 -> 进世界 流程, 按客户端内置 ServerDummy (vtable 0x5f4fd0) 的回包逐字节复刻。
// ServerDummy 是客户端里自带的离线假服务器: 命令表在 0x41bf1a 注册,
//   C0x100->0x41c460  C0x220->0x41c490  C0x230->0x41c520  C0x410->0x41c6c0
// 它回给客户端的包结构就是客户端能吃下的结构, 所以照抄最稳。

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"

	"sa2srv/internal/cmd"
	"sa2srv/internal/mbcs"
	"sa2srv/internal/proto"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

const (
	charEntrySize   = 0xE3 // OnCharList 0x428cd8: add edi,0xe3
	charLoginSize   = 0xFD // ServerDummy 0x41c606: push 0xfd
	objPosSize      = 0x18
	defaultCharID   = 0x15 // ServerDummy 用的角色 id
	defaultCharName = "DummyPlayer"
	charListNameOff = 0x34
	charListNameMax = 20
)

func put16(b []byte, off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
func put32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }

func send(c server.Conn, command uint16, pl []byte) error {
	err := server.Send(c, &proto.Frame{Cmd: command, Payload: pl})
	if c != nil {
		mirrorBattle(c, command, pl)
	}
	return err
}

// handleFlow 返回 handled=false 时交给旧 switch。
func (h *Handler) handleFlow(c server.Conn, f *proto.Frame) (bool, error) {
	if ok, err := h.onBattlePacket(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if ok, err := h.onNpcPacket(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if ok, err := h.onItemUse(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if ok, err := h.onItemPacket(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if ok, err := h.onPartyPacket(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if ok, err := h.onTradePacket(c, f.Cmd, f.Payload); ok {
		return true, err
	}
	if f.Cmd == 0x0800 {
		return true, onOptions(c, f.Payload)
	}
	if f.Cmd == 0x0530 {
		return true, onSkillInfo(c, f.Payload)
	}
	switch f.Cmd {
	case cmd.CV_Version:
		return true, h.onVersion(c, f)
	case cmd.CV_Reconnect:
		// 选服界面状态探测: 连上发 C0x0020, 等 S0x0020 OnLoginCnt (0x414600 -> 0x4f4fc0)。
		// 负载 u16 cnt + cnt*0x1C: 记录 +0x14 u32 在线人数 (<0 Stop, <200 档1, <1200 档2 ... 0x4f4600), +0x18 u32 上限
		log.Printf("sa2: server-state probe(0x20) from %s", c.RemoteAddr())
		pl := make([]byte, 2+0x1c)
		put16(pl, 0, 1)
		put32(pl, 2+0x14, 10)
		put32(pl, 2+0x18, 2000)
		return true, send(c, cmd.SV_LoginCnt, pl)
	case cmd.CV_Map:
		return true, h.onMapReq(c, f)
	case cmd.CV_Login:
		return true, h.onLogin(c, f)
	case cmd.CV_CharCreate:
		return true, h.onCharCreate(c, f)
	case cmd.CV_CharDelete:
		return true, h.onCharDelete(c, f)
	case cmd.CV_CharList:
		return true, h.onCharList(c, f)
	case cmd.CV_CharLogin:
		return true, h.onCharLogin(c, f)
	case cmd.CV_Move:
		return true, h.onMove(c, f)
	case cmd.CV_CharParam:
		return true, send(c, cmd.SV_CharParam, []byte{0, 0}) // 无变化
	case cmd.CV_Chat:
		return true, h.onChat(c, f)
	case cmd.CV_MoveDelta:
		return true, h.onMoveDelta(c, f)
	case cmd.CV_WorldTime:
		if err := send(c, cmd.SV_WorldTime, worldTime()); err != nil {
			return true, err
		}
		return true, h.maybeHUD(c)
	}
	return false, nil
}

// C 0x0010: char[8] 协议版本 ("2.138", 常量 double 0x40011a9fc0000000 用 "%0.3f" 打印) + char[8] 客户端版本。
func (h *Handler) onVersion(c server.Conn, f *proto.Frame) error {
	cstr := func(b []byte) string {
		for i, x := range b {
			if x == 0 {
				return string(b[:i])
			}
		}
		return string(b)
	}
	p, v := "", ""
	if len(f.Payload) >= 8 {
		p = cstr(f.Payload[:8])
	}
	if len(f.Payload) >= 16 {
		v = cstr(f.Payload[8:16])
	}
	s := getSess(c.RemoteAddr())
	s.kind, s.kindSet = detectClient(p), true
	log.Printf("sa2: version proto=%q client=%q -> protocol %s (mode %s) from %s", p, v, s.kind, ClientMode, c.RemoteAddr())
	return sendVersionOK(c)
}

// S 0x0010: u32 res=0 (非0 弹错误框 0xb0/0xb1/0xec), u32 seed, u16 codecType=0 (恒等 codec)。
// res==0 时客户端立刻发 0x0100 登录包。
func sendVersionOK(c server.Conn) error {
	pl := make([]byte, 10)
	put32(pl, 0, 0)
	put32(pl, 4, 0x12345678)
	put16(pl, 8, 0)
	return send(c, cmd.SV_Version, pl)
}

// C 0x0100: u32 x, u8 loginType, u16 idLen, u16 pwLen, id[idLen], pw[pwLen] (loginType==3 时无 pw)。
func parseLogin(b []byte) (typ byte, id, pw string, ok bool) {
	if len(b) < 9 {
		return 0, "", "", false
	}
	typ = b[4]
	il := int(binary.LittleEndian.Uint16(b[5:7]))
	pl := int(binary.LittleEndian.Uint16(b[7:9]))
	if 9+il > len(b) {
		return typ, "", "", false
	}
	id = string(b[9 : 9+il])
	if 9+il+pl <= len(b) {
		pw = string(b[9+il : 9+il+pl])
	}
	return typ, id, pw, true
}

// S 0x0100 结果码 (UI OnLogin 0x415d40 跳转表 0x415fb8; CN 0x419670 跳转表 0x419a00)。
// 自定义文本错误: UI result=5, CN result=6 (CN 的 5 是内置实名认证提示), 见 clientKind.loginCustomCode。
const (
	OnLoginFail uint32 = 0 // 提示文本 0x12a
	OnLoginOK   uint32 = 1 // 1/2 都走成功分支
	OnLoginMsg4 uint32 = 4 // 提示文本 0x126
)

// loginCustom: u32 code + u16 len + 文本 (按客户端编码)。
func loginCustom(c server.Conn, msg string) []byte {
	t := mbcs.EncodeAs(connKind(c).loginTextCharset(), msg)
	pl := make([]byte, 6+len(t))
	put32(pl, 0, connKind(c).loginCustomCode())
	put16(pl, 4, uint16(len(t)))
	copy(pl[6:], t)
	return pl
}

func (h *Handler) onLogin(c server.Conn, f *proto.Frame) error {
	typ, id, pw, ok := parseLogin(f.Payload)
	if !ok {
		return send(c, cmd.SV_Login, loginCustom(c, "登录包错误 bad login packet"))
	}
	var err error
	if h.saac != nil {
		_, err = h.saac.Verify(id, pw)
	}
	log.Printf("sa2: login id=%q type=%d pwLen=%d err=%v from %s", id, typ, len(pw), err, c.RemoteAddr())
	switch {
	case err == nil:
		getSess(c.RemoteAddr()).account = id
		pl := make([]byte, 4)
		put32(pl, 0, OnLoginOK)
		return send(c, cmd.SV_Login, pl)
	case errors.Is(err, saac.ErrNotFound):
		return send(c, cmd.SV_Login, loginCustom(c, "账号不存在 no such account"))
	case errors.Is(err, saac.ErrBadPwd):
		return send(c, cmd.SV_Login, loginCustom(c, "密码错误 wrong password"))
	}
	return send(c, cmd.SV_Login, loginCustom(c, "服务器错误 server error"))
}

// S 0x0220: u16 cnt + 每条 0xE3 字节。已知字段: +0 u16 槽位, +2 u32 charId, +0x34 名字 (MBCS, 0 结尾)。
// 名字偏移: UI 0x4ee1a1 / CN 0x4f7311 都从 entry+0x34 读 (旧代码写 +0x36 → 首字节为 0, 列表名字为空)。
// 槽位缓冲至少 2 个 (ServerDummy 0x1c8 = 2+2*0xe3)。账号无角色时 cnt=0 → 客户端应进入建角界面。
func (h *Handler) onCharList(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	var chars []saac.Char
	if h.saac != nil && s.account != "" {
		chars, _ = h.saac.ListChars(s.account)
	}
	slots := len(chars)
	if slots < 2 {
		slots = 2
	}
	pl := make([]byte, 2+slots*charEntrySize)
	put16(pl, 0, uint16(len(chars)))
	for i, ch := range chars {
		e := pl[2+i*charEntrySize:]
		put16(e, 0, uint16(ch.Slot))
		// 真机抓包: 选角后客户端发回的 0x0230 id = 条目 +2 的 u32 (原来写在 +4 时收到 0x01000000)
		put32(e, 2, ch.ID)
		putText(c, e[charListNameOff:charListNameOff+charListNameMax], charListNameMax, ch.Name)
	}
	log.Printf("sa2: charlist acct=%q -> %d chars", s.account, len(chars))
	return send(c, cmd.SV_CharList, pl)
}

// C 0x0200 建角 (0x40d080): u16 slot + 0x82 字节结构; 结构 +0x32 = 名字 (MBCS, 80 字节) → 负载 +0x34。
// 其余字段 (外形/属性点?) 未解。S 0x0200 OnCharCreate (0x410d90): u32 result, 1 = 成功 (0x4ed220 分支)。
func (h *Handler) onCharCreate(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	res := make([]byte, 4)
	if len(f.Payload) < 2 || h.saac == nil || s.account == "" {
		put32(res, 0, 2)
		return send(c, cmd.SV_CharCreate, res)
	}
	slot := int(binary.LittleEndian.Uint16(f.Payload))
	name := "Player"
	if len(f.Payload) > 0x34 {
		nb := f.Payload[0x34:]
		if len(nb) > 80 {
			nb = nb[:80]
		}
		for i, b := range nb {
			if b == 0 {
				nb = nb[:i]
				break
			}
		}
		if len(nb) > 0 {
			name = decText(c, nb)
		}
	}
	m, x, y := spawnPoint()
	ch := &saac.Char{Account: s.account, Slot: slot, Name: name, Map: m, X: x, Y: y}
	if err := h.saac.CreateChar(ch); err != nil {
		log.Printf("sa2: char create fail: %v", err)
		put32(res, 0, 2)
		return send(c, cmd.SV_CharCreate, res)
	}
	log.Printf("sa2: char create acct=%q slot=%d name=%q id=0x%x spawn=%d(%d,%d)", s.account, slot, name, ch.ID, m, x, y)
	put32(res, 0, 1)
	if err := send(c, cmd.SV_CharCreate, res); err != nil {
		return err
	}
	return h.onCharList(c, f)
}

func (h *Handler) onCharDelete(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	if len(f.Payload) >= 2 && h.saac != nil && s.account != "" {
		_ = h.saac.DeleteChar(s.account, int(binary.LittleEndian.Uint16(f.Payload)))
	}
	if err := send(c, cmd.SV_CharDelete, nil); err != nil {
		return err
	}
	return h.onCharList(c, f)
}

// C 0x0230 u32 charId -> S 0x0230 (0xFD) + S 0x0A00 地图 + S 0x0410 位置 + S 0x0900。
// (ServerDummy 不发 0x0A00, 它在本地直接读 mapdata; 真服务器需发, 否则客户端无地图/视点落在 0,0)
func (h *Handler) onCharLogin(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	var id uint32 = defaultCharID
	if len(f.Payload) >= 4 {
		id = binary.LittleEndian.Uint32(f.Payload)
	}
	m, x, y := spawnPoint()
	ch := &saac.Char{ID: id, Name: "Dummy", Map: m, X: x, Y: y}
	if h.saac != nil && s.account != "" {
		if l, err := h.saac.ListChars(s.account); err == nil {
			for i := range l {
				if l[i].ID == id {
					ch = &l[i]
				}
			}
		}
	}
	if ch.Map == 0 || (ch.X == 0 && ch.Y == 0) {
		ch.Map, ch.X, ch.Y = m, x, y
	}
	s.char = ch
	// 存库的等级/EXP/场景HP (0 = 新角色/旧库, 用默认值)。EXP 0 合法, 但等级 0 不合法, 所以以 Level 判断是否已存过。
	s.level, s.exp, s.fieldHP = baseLevel, baseExp, fieldHPInit
	if ch.Level > 0 {
		s.level, s.exp = ch.Level, ch.Exp
	}
	if ch.HP > 0 {
		s.fieldHP = min(ch.HP, fieldHPMax)
	}
	s.expInit = true
	s.hudSent = false
	log.Printf("sa2: char login id=0x%x name=%q map=%d (%d,%d) lv=%d exp=%d hp=%d", ch.ID, ch.Name, ch.Map, ch.X, ch.Y, s.level, s.exp, s.fieldHP)
	loadInventory(s)
	cl := charLoginData(ch.ID, fitText(c, ch.Name, 15), s.level, s.exp, s.fieldHP)
	put32(cl, 0x32, uint32(s.gold))     // 石币
	putCharStatsTot(cl, playerStats(s)) // 总数 = 基础 + 装备
	if err := send(c, cmd.SV_CharLogin, cl); err != nil {
		return err
	}
	// 不在这里发 0x0A00: 真机崩溃 EIP 0x504E0E —— OnMap(0x416290) 在地图表 0x6a75dc 查不到 mapId 时
	// 用全局当前地图对象 [0x6a75c4] 当 this 调 0x504e00, 进世界场景尚未建好时它为 NULL → 读 [0+0x194] 崩。
	// ServerDummy 也从不发 0x0A00; 地图由客户端 C0x0A03 请求, 见 onMapReq。
	if err := send(c, cmd.SV_ObjPos, objPos(ch.ID, uint16(ch.Map), uint16(ch.X), uint16(ch.Y), 4, 30002)); err != nil {
		return err
	}
	if err := send(c, cmd.SV_WorldTime, worldTime()); err != nil {
		return err
	}
	h.enterWorld(c, s)
	sendAllPets(c, s)
	return nil
}

// C 0x0A03 SendMap(8 字节) -> S 0x0A03 OnMap(0x40d770): u16 ret, u16 mapId [, 数据]。
// ret==0: +4 起是整份 mapdata 文件, 交 0x427fb0 按文件头 +0xA mapId 缓存 (ServerDummy 0x419550 就是本地读文件喂它);
//
//	但 map100 = 72502 字节 > 单包 u16 上限, 发不了。
//
// ret!=0: 0x422b10 回放加载期间排队的包 (0x428d00 排队除 0x900/0xa03 外所有包), 用 mapId 比较/切图 (0x501b10)。
// 这里用 ret=1 + mapId, 让客户端用本地 mapdata。
func (h *Handler) onMapReq(c server.Conn, f *proto.Frame) error {
	s := getSess(c.RemoteAddr())
	m := SpawnMap
	if s.char != nil {
		m = s.char.Map
	}
	// C0A03: CN 6 字节 (u16+0, u32+2), UI 8 字节 (u16+0, 对齐, u32+4)。目前只记录, 不依赖负载。
	k := connKind(c)
	want := 6
	if k == clientUI {
		want = 8
	}
	if len(f.Payload) != want {
		log.Printf("sa2: map req len %d != %d for %s client", len(f.Payload), want, k)
	}
	log.Printf("sa2: map req % x (%s) -> map %d", f.Payload, k, m)
	pl := make([]byte, 4)
	put16(pl, 0, 1)
	put16(pl, 2, uint16(m))
	if err := send(c, cmd.SV_MapRet, pl); err != nil {
		return err
	}
	return h.maybeHUD(c)
}

// maybeHUD: 进图后首个 C 0x0900 (或 0x0A03) 时发一次 S 0x00E0 flags=4 → 0x40d2c0 创建 HUD 窗口 0x1388/0x1387。
// 此时场景已在 (客户端只在场景里发 0x0900); 即使仍在加载, 0x428d00 只放行 0x0900/0x0A03, 其余排队稍后处理。
// 0x0A03 客户端只是偶尔发 (本地地图缓存命中时不发), 不能依赖。
func (h *Handler) maybeHUD(c server.Conn) error {
	s := getSess(c.RemoteAddr())
	if s.char == nil || s.hudSent {
		return nil
	}
	s.hudSent = true
	log.Printf("sa2: HUD enable (S 0x00E0 flags=4) for %q", s.char.Name)
	if err := send(c, cmd.SV_UIFlags, []byte{4, 0, 0, 0}); err != nil {
		return err
	}
	spawnNpcs(c, s.char.Map)
	_ = sendInventory(c, s)
	if connKind(c) == clientCN {
		_ = sysMsg(c, 0xffffff00, "欢迎来到石器时代2！中文测试：国服协议已启用。")
	} else {
		_ = sysMsg(c, 0xffffff00, "Welcome to Stone Age 2 (sa2srv, UI protocol).")
	}
	return nil
}

// S 0x0A00 OnMap (0x416290): +0 u16 mapId, +0xC u16 layer, +0x12 UTF-16 名字 (0 结尾);
// +0..+0x11 整段被 0x504e00 拷入地图对象 +0x188。+2..+0xB 未确认, 这里放 x,y,gridW,gridH 作为猜测。
func mapPacket(m, layer, x, y int) []byte {
	w, hh, _ := mapGrid(m)
	name := utf16z(fmt.Sprintf("map%05d", m))
	b := make([]byte, 0x12+len(name))
	put16(b, 0, uint16(m))
	put16(b, 2, uint16(x))
	put16(b, 4, uint16(y))
	put16(b, 6, uint16(w))
	put16(b, 8, uint16(hh))
	put16(b, 0xc, uint16(layer))
	copy(b[0x12:], name)
	return b
}

// charLoginData 复刻 ServerDummy 0x41c54d..0x41c629 写入的值 (偏移已换算到负载起点)。
// 语义未逐个确认, 只保证与客户端自带假服务器一致。
// lv/exp/hp: +0x04 u8 等级, +0x05 u32 EXP, +0x11 u16 场景 HP (最大 +0x13 = fieldHPMax)。
func charLoginData(id uint32, name []byte, lv, exp, hp int) []byte {
	b := make([]byte, charLoginSize)
	put32(b, 0x00, id)
	b[0x04] = byte(lv)
	put32(b, 0x05, uint32(exp))
	// 状态窗 "经验值 cur / NEXT" = +0x09 / +0x0d (国服真机 2026-10-17: 旧固定值显示 1000 / 3000)。
	put32(b, 0x09, uint32(exp))
	put32(b, 0x0d, uint32(playerNext(lv)))
	for off, v := range map[int]uint16{0x11: uint16(hp), 0x13: fieldHPMax, 0x1d: 100} {
		put16(b, off, v)
	}
	putCharStats(b, statsAt(lv)) // 攻/防/敏/体/灵 (charstats.go)
	b[0x27], b[0x28], b[0x29], b[0x2a] = 50, 30, 60, 70
	b[0x2d] = 2
	put32(b, 0x32, 0x41a)
	// +0x40 u16 → player+0x982: 0 时 0x230 UI 分发 (0x5547a5→0x553df0(0)) 禁用指南针 HUD 的聊天按钮 0x13885 (vtbl+0x10c);
	// 非 0 → 启用 (vtbl+0x114)。真机内存确认 player+0x982=0, 名字在 +0x984 (=payload+0x42)。
	put16(b, 0x40, 1)
	copy(b[0x42:0x42+15], name) // 16 字节区, 保留终止 0 (调用方已按客户端编码)
	return b
}

// objPos 0x18 字节: +0 u32 id, +4 u16 (地图?), +6 u16 x, +8 u16 y, +0xa u8 (方向?), +0xc u16 (形象 id?)。
func objPos(id uint32, m, x, y uint16, dir byte, gfx uint16) []byte {
	b := make([]byte, objPosSize)
	put32(b, 0, id)
	put16(b, 4, m)
	put16(b, 6, x)
	put16(b, 8, y)
	b[0xa] = dir
	put16(b, 0xc, gfx)
	return b
}

// worldTime 10 字节: u16 1, u16 15, u16 0, u32 60 (ServerDummy 0x41c675..0x41c68a)。
func worldTime() []byte {
	b := make([]byte, 10)
	put16(b, 0, 1)
	put16(b, 2, 15)
	put16(b, 4, 0)
	put32(b, 6, 60)
	return b
}
