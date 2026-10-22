package handler

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"

	"sa2srv/internal/cmd"
	"sa2srv/internal/proto"
	"sa2srv/internal/saac"
	"sa2srv/internal/server"
)

// Handler 默认实现 + sqlite 账号验证。
type Handler struct {
	saac *saac.Store
}

func New(st *saac.Store) *Handler { store = st; return &Handler{saac: st} }

func (h *Handler) Handle(c server.Conn, f *proto.Frame) error {
	if !server.Legacy {
		if ok, err := h.handleFlow(c, f); ok {
			return err
		}
	}
	switch f.Cmd {
	case cmd.C_Ping:
		return h.handlePing(c, f)
	case cmd.C_Login:
		return h.handleLogin(c, f)
	case cmd.C_Register:
		return h.handleRegister(c, f)
	case cmd.C_Logout:
		log.Printf("sa2: logout seq=%d from %s", f.Seq, c.RemoteAddr())
		return nil
	case cmd.C_Move:
		return h.handleMove(c, f)
	case cmd.C_Chat:
		return h.handleChat(c, f)
	case cmd.C_NpcTalk:
		return h.handleNpcTalk(c, f)
	case cmd.C_PetCmd:
		return h.handlePetCmd(c, f)
	case cmd.CV_PetBattle:
		return h.handlePetBattle(c, f)
	case cmd.CV_PetRole:
		return h.handlePetRole(c, f)
	case cmd.C_BattleCmd:
		return h.handleBattleCmd(c, f)
	case cmd.C_ItemUse:
		return h.handleItemUse(c, f)
	}
	log.Printf("sa2: cmd=0x%04x seq=%d len=%d from %s", f.Cmd, f.Seq, f.Len, c.RemoteAddr())
	return nil
}

func (h *Handler) OnClose(c server.Conn) {
	h.leaveWorld(c)
	dropSess(c.RemoteAddr())
	log.Printf("sa2: conn closed: %s", c.RemoteAddr())
}

func (h *Handler) handlePing(c server.Conn, f *proto.Frame) error {
	return server.Send(c, &proto.Frame{
		Seq: f.Seq,
		Cmd: cmd.S_Pong,
	})
}

// LoginResult 状态码 (1字节):
//
//	0 = OK
//	1 = 用户不存在
//	2 = 密码错误
//	3 = 其它错误
const (
	LoginOK         byte = 0
	LoginNoSuchUser byte = 1
	LoginBadPwd     byte = 2
	LoginErr        byte = 3
)

// RegResult 状态码 (1字节):
//
//	0 = OK
//	1 = 用户名已存在
//	2 = 参数错误
//	3 = 其它错误
const (
	RegOK     byte = 0
	RegDup    byte = 1
	RegBadArg byte = 2
	RegErr    byte = 3
)

func (h *Handler) handleRegister(c server.Conn, f *proto.Frame) error {
	name, pwd, ok := parseLoginPayload(f.Payload)
	if !ok || name == "" {
		return server.Send(c, &proto.Frame{
			Seq:     f.Seq,
			Cmd:     cmd.S_RegResult,
			Payload: []byte{RegBadArg},
		})
	}
	_, err := h.saac.Create(name, pwd)
	status := RegOK
	switch {
	case err == nil:
		status = RegOK
	case errors.Is(err, saac.ErrDup):
		status = RegDup
	default:
		status = RegErr
		log.Printf("sa2: register err: %v", err)
	}
	log.Printf("sa2: register name=%q status=%d from %s", name, status, c.RemoteAddr())
	return server.Send(c, &proto.Frame{
		Seq:     f.Seq,
		Cmd:     cmd.S_RegResult,
		Payload: []byte{status},
	})
}

func (h *Handler) handleLogin(c server.Conn, f *proto.Frame) error {
	if h.saac == nil {
		return errors.New("saac 未初始化")
	}
	name, pwd, ok := parseLoginPayload(f.Payload)
	if !ok {
		return server.Send(c, &proto.Frame{
			Seq:     f.Seq,
			Cmd:     cmd.S_LoginResult,
			Payload: []byte{LoginErr},
		})
	}
	_, err := h.saac.Verify(name, pwd)
	status := LoginOK
	switch {
	case err == nil:
		status = LoginOK
	case errors.Is(err, saac.ErrNotFound):
		status = LoginNoSuchUser
	case errors.Is(err, saac.ErrBadPwd):
		status = LoginBadPwd
	default:
		status = LoginErr
		log.Printf("sa2: login verify err: %v", err)
	}
	log.Printf("sa2: login name=%q status=%d from %s", name, status, c.RemoteAddr())
	return server.Send(c, &proto.Frame{
		Seq:     f.Seq,
		Cmd:     cmd.S_LoginResult,
		Payload: []byte{status},
	})
}

// parseLoginPayload 解析 [u16 nameLen | name | u16 pwdLen | pwd]。
func parseLoginPayload(b []byte) (name, pwd string, ok bool) {
	if len(b) < 2 {
		return "", "", false
	}
	nameLen := int(binary.LittleEndian.Uint16(b[0:2]))
	if 2+nameLen+2 > len(b) {
		return "", "", false
	}
	name = string(b[2 : 2+nameLen])
	pwdLen := int(binary.LittleEndian.Uint16(b[2+nameLen : 4+nameLen]))
	if 4+nameLen+pwdLen > len(b) {
		return name, "", true // pwd 截断也照返回 (上层按 status 处理)
	}
	return name, string(b[4+nameLen : 4+nameLen+pwdLen]), true
}

func (h *Handler) handleMove(c server.Conn, f *proto.Frame) error {
	if len(f.Payload) < 4 {
		return fmt.Errorf("move: short payload %d", len(f.Payload))
	}
	x := binary.LittleEndian.Uint16(f.Payload[0:2])
	y := binary.LittleEndian.Uint16(f.Payload[2:4])
	log.Printf("sa2: move to (%d,%d) from %s", x, y, c.RemoteAddr())
	return nil
}

// --- 聊天 / NPC / 宠物 / 战斗 / 物品: 占位 handler ---
// payload 结构是从 SA1 / 客户端逆向猜的 (未完全确认, 标 TODO).
// SA2 客户端真连时, 按 RUDP/服务端抓包修正。

func (h *Handler) handleChat(c server.Conn, f *proto.Frame) error {
	// TODO SA2 实际格式: u8 channel | u16 len | UTF-16LE text (验证)
	if len(f.Payload) < 1 {
		return nil
	}
	log.Printf("sa2: chat channel=%d len=%d from %s", f.Payload[0], len(f.Payload)-1, c.RemoteAddr())
	return nil
}

func (h *Handler) handleNpcTalk(c server.Conn, f *proto.Frame) error {
	// TODO 验证 NPC id 字段宽度 (u32/u64)
	if len(f.Payload) < 4 {
		return nil
	}
	npcID := binary.LittleEndian.Uint32(f.Payload[0:4])
	log.Printf("sa2: npc talk id=%d from %s", npcID, c.RemoteAddr())
	return nil
}

func (h *Handler) handlePetCmd(c server.Conn, f *proto.Frame) error {
	// TODO 验证 action + slot 字段宽度
	if len(f.Payload) < 2 {
		return nil
	}
	log.Printf("sa2: pet cmd action=%d slot=%d from %s", f.Payload[0], f.Payload[1], c.RemoteAddr())
	return nil
}

// C 0x0465 SendPetBattle (0x411c80): 1 字节出战数设置 (player+0x9f4)。
// Client log "SendPetBattle cnt = %d". Selecting a battle pet (Pal UI).
func (h *Handler) handlePetBattle(c server.Conn, f *proto.Frame) error {
	cnt := 1
	if len(f.Payload) >= 1 {
		cnt = int(f.Payload[0])
	}
	if cnt < 1 {
		cnt = 1
	}
	if cnt > 5 {
		cnt = 5
	}
	s := getSess(c.RemoteAddr())
	s.battleCnt = cnt
	// 0x411c80 日志 "SendPetBattle cnt = %d": 字节 = player+0x9f4 出战数设置。
	log.Printf("sa2: SendPetBattle cnt=%d from %s", cnt, c.RemoteAddr())
	return nil
}

// C 0x1463 SendPetRole (0x413250, Pal → Role → Rest/Battle): u32 petId, u32 role (0=Battle, 1=Rest)。
func (h *Handler) handlePetRole(c server.Conn, f *proto.Frame) error {
	if len(f.Payload) < 8 {
		return nil
	}
	id := binary.LittleEndian.Uint32(f.Payload)
	role := binary.LittleEndian.Uint32(f.Payload[4:])
	log.Printf("sa2: SendPetRole id=0x%x role=%d (0=Battle 1=Rest)", id, role)
	return setPetRole(c, getSess(c.RemoteAddr()), id, role)
}

func (h *Handler) handleBattleCmd(c server.Conn, f *proto.Frame) error {
	// TODO 验证 cmd + target 字段
	if len(f.Payload) < 1 {
		return nil
	}
	log.Printf("sa2: battle cmd=%d from %s", f.Payload[0], c.RemoteAddr())
	return nil
}

func (h *Handler) handleItemUse(c server.Conn, f *proto.Frame) error {
	// TODO 验证 slot 字段
	if len(f.Payload) < 1 {
		return nil
	}
	log.Printf("sa2: item use slot=%d from %s", f.Payload[0], c.RemoteAddr())
	return nil
}
