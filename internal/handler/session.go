package handler

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"sa2srv/internal/server"
	"unicode/utf16"

	"sa2srv/internal/saac"
)

// 每连接会话 (账号 + 当前角色)。
type session struct {
	items                               []*saac.Item // 背包 (shop.go)
	gold                                int
	shopOpen                            bool
	account                             string
	kind                                clientKind // 客户端协议 (C0010 决定)
	kindSet                             bool
	expInit                             bool   // exp/level 已在选角时初始化 (EXP 0 是合法值, 不能当哨兵)
	syncTok                             uint32 // 最近一次 S0BD0 的 token
	syncWait                            bool   // 正在等 C0BD0 回显
	char                                *saac.Char
	hudSent                             bool   // S 0x00E0 已发
	dlg                                 uint32 // 当前打开的 NPC 对话页 (0 = 无)
	inBattle, rewarded, syncSent, ended bool
	orders                              int      // 本回合收到的 C 0x0b20 数 (无宠=角色 2 槽; 有宠=角色+宠)
	orderActors                         []uint32 // 每条 C 0x0b20 的 actor id (+0)
	hasPet                              bool
	pets                                []saac.Pet     // 本角色宠物 (DB, slot 0..4)
	petHP                               int            // 出战宠战斗中 HP (第一只, 兼容)
	petHPs                              map[uint32]int // 各出战宠战斗中 HP
	battleCnt                           int            // C 0x0465 出战数上限 (0→1)
	exp                                 int            // 角色 EXP (0x0230 +0x05, 0x0420 param 21)
	fieldHP                             int            // 场景 HP (0x0230 +0x11 u16, 0x0420 param 4); 0 = 未初始化 (=fieldHPInit)   // /fight 测试战斗
	monHP, syncN, hp                    int
	mon                                 *monster          // 本场第一只野怪 (数据表); nil = 无数据表
	foes                                []*foe            // 本场全部野怪 (battleturn.go)
	orderTgt                            map[uint32]uint32 // 本回合 C0B20: 行动者 id → 目标 id
	orderItem, orderItemTgt             uint32            // 本回合角色指令 = 用道具 (C0B20 act 0x9a): 物品 id, 目标 (角色/宠物 id)
	queue                               []turnStep        // 本回合剩余动作
	orderAct                            map[uint32]byte   // 本回合 C0B20: 行动者 id → 指令 act (+8)
	guard                               [2]bool           // 本回合防御中: [0] 角色 [1] 宠物
	escaped                             bool              // 逃跑成功, 等动作播完再结算
	escTries                            int               // 本场逃跑次数 (每次失败 +成功率)
	level                               int               // 当前等级 (0x0230 +0x04 发 3; 之后 0x0420 增量)
	warpHold                            bool              // 刚传送落在传送格上: 离开前不再触发 (warp.go)
	encSteps                            int               // 上次遇敌后走的步数 (encounter.go)
	petSkill                            *petSkill         // 本回合出战宠选的技能 (nil = 普通攻击)
	petSkillTgt                         uint32
	allies                              []*ally // 组队战斗: 队员单位 (partybattle.go)
	// 组队战斗, 队员自己操作 (partyctl.go)
	battleHost  *session    // 队员: 正在参加的队长战斗
	members     []*bMember  // 队长: 进了战斗画面的队员
	bconn       server.Conn // 队长: 自己的连接 (超时回调用)
	turnPending bool        // 队长: 等队员指令
	turnGen     int
	noDrops     bool     // rewardWin 不掷掉落 (组队时统一分配)
	leaveAfter  []uint32 // 逃跑的队员, 战后离队
}

var (
	sessMu sync.Mutex
	sess   = map[string]*session{}
)

func getSess(addr string) *session {
	sessMu.Lock()
	defer sessMu.Unlock()
	s := sess[addr]
	if s == nil {
		s = &session{}
		sess[addr] = s
	}
	return s
}

func dropSess(addr string) { sessMu.Lock(); delete(sess, addr); sessMu.Unlock() }

// 出生点配置。MapDir = 客户端 mapdata 目录 (读头部 gridW/gridH 取中心)。
var (
	MapDir   = "../Client/mapdata"
	SpawnMap = 100 // 新手村 (ServerDummy 0x410 包 +4 也用 100)
	SpawnX   = -1  // <0 = 用地图中心
	SpawnY   = -1
)

// mapGrid 读 mapdata/%05d.bin 头: +0xA mapid, +0xC gridW, +0xE gridH (子格)。
func mapGrid(id int) (w, h int, err error) {
	f, err := os.Open(filepath.Join(MapDir, fmt.Sprintf("%05d.bin", id)))
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var hdr [16]byte
	if _, err := f.Read(hdr[:]); err != nil {
		return 0, 0, err
	}
	if string(hdr[:3]) != "MAP" {
		return 0, 0, fmt.Errorf("map %d: bad magic", id)
	}
	return int(binary.LittleEndian.Uint16(hdr[0xC:])), int(binary.LittleEndian.Uint16(hdr[0xE:])), nil
}

// spawnPoint 返回出生点 (地图中心, 读不到地图时用 ServerDummy 的 150,150)。
func spawnPoint() (m, x, y int) {
	m, x, y = SpawnMap, 150, 150
	if w, h, err := mapGrid(m); err == nil {
		x, y = w/2, h/2
	}
	if SpawnX >= 0 {
		x = SpawnX
	}
	if SpawnY >= 0 {
		y = SpawnY
	}
	return
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}
