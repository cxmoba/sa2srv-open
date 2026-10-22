// Package mapdata SA2 客户端 mapdata/*.bin 解析 (国服/欧版同格式, 版本 0x105)。
//
// 格式 (2026-10-21 按国服/欧版文件复核):
//
//	+0x00 "MAP\0"
//	+0x04 u32  分段起点 (= 40 + W×H, W/H 是子格数)
//	+0x08 u16  版本 0x105
//	+0x0A u16  地图号
//	+0x0C u16  W (子格), +0x0E u16 H (子格)
//	+0x10 u32  时间戳?  +0x14 u16 区域号 (战斗背景/遇敌区)  其余未明
//	+0x28      地砖表: (W/2)×(H/2) 个 u32, 行优先; 高 16 位 = 地砖号 (chipinfo.bin 的键), 低 16 位 未明 (光照/朝向?)
//	           (以前写死 125×125, 只对 250×250 子格的图 (如 00100) 对; 其他尺寸要按头算)
//	分段起点起: 4 字节 tag + u32 段长 (含这 8 字节) + u16 版本 + u32 条数 + 条目, 依次:
//	  OBJ1 10 字节 {u16 x, u16 y, u16 物件图号, u16, u16} (地砖坐标)
//	  DINO / BATT / EVEN / LAYE / PLAC  未解
//	  JUMP 8 字节 {u16 x, u16 y, u16 目标地图, u16 目标落点号}   出入口格 (子格坐标)
//	  JUM2 11 字节 {u16 落点号, u16 x, u16 y, u32 ?, u8 朝向}    本图落点
//
// 服务端目前只用头 (handler: 出生点/边界/战斗背景); 本包供碰撞 (未启用) 和传送表 (warp 加载) 用。
package mapdata

import (
	"encoding/binary"
	"errors"
	"os"
)

// WalkableService 服务端碰撞表: gid 启发式 + obj 障碍。
type WalkableService struct {
	Strict bool // 未知 gid 默认不可走 (上线前开)
}

// Jump 出入口格 (子格坐标)。
type Jump struct {
	X, Y        int
	ToMap, ToID int
}

// Land 落点 (子格坐标)。
type Land struct {
	ID, X, Y, Dir int
}

// Map 地砖网格 (Size×Rows 个地砖, 每格 = 2×2 子格) + 物件 + 出入口。
type Map struct {
	ID, W, H   int // 地图号, 子格宽高
	Size, Rows int // 地砖宽高 (W/2, H/2); Size 名字沿用旧版 (旧版固定 125)
	Area       int
	Tiles      []uint16 // 地砖号
	Low16      []uint16 // 低 16 位 (未解)
	Objs       []uint16 // 物件图号, 0 = 空
	Jumps      []Jump
	Lands      []Land
	Sections   map[string]int // tag → 段长
}

var le = binary.LittleEndian

// Load 读 mapdata/*.bin。
func Load(path string) (*Map, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(buf)
}

// Parse 解析整份文件。
func Parse(buf []byte) (*Map, error) {
	if len(buf) < 40 || string(buf[:4]) != "MAP\x00" {
		return nil, errors.New("mapdata: bad magic")
	}
	m := &Map{ID: int(le.Uint16(buf[0xa:])), W: int(le.Uint16(buf[0xc:])), H: int(le.Uint16(buf[0xe:])),
		Area: int(le.Uint16(buf[0x14:])), Sections: map[string]int{}}
	m.Size, m.Rows = m.W/2, m.H/2
	n := m.Size * m.Rows
	if 40+n*4 > len(buf) {
		return nil, errors.New("mapdata too short")
	}
	m.Tiles, m.Low16, m.Objs = make([]uint16, n), make([]uint16, n), make([]uint16, n)
	for i := 0; i < n; i++ {
		v := le.Uint32(buf[40+i*4:])
		m.Tiles[i], m.Low16[i] = uint16(v>>16), uint16(v)
	}
	p := int(le.Uint32(buf[4:]))
	if p < 40+n*4 {
		p = 40 + n*4
	}
	for p+14 <= len(buf) {
		tag, ln := string(buf[p:p+4]), int(le.Uint32(buf[p+4:]))
		if ln < 8 || p+ln > len(buf) {
			break
		}
		if _, dup := m.Sections[tag]; dup { // 尾部有一组空段重复, 只取第一个
			p += ln
			continue
		}
		m.Sections[tag] = ln
		cnt, body := 0, buf[p+14:p+ln]
		if ln >= 14 {
			cnt = int(le.Uint32(buf[p+10:]))
		}
		switch tag {
		case "OBJ1":
			for i := 0; i < cnt && i*10+10 <= len(body); i++ {
				x, y := int(le.Uint16(body[i*10:])), int(le.Uint16(body[i*10+2:]))
				if x < m.Size && y < m.Rows {
					m.Objs[y*m.Size+x] = le.Uint16(body[i*10+4:])
				}
			}
		case "JUMP":
			for i := 0; i < cnt && i*8+8 <= len(body); i++ {
				e := body[i*8:]
				m.Jumps = append(m.Jumps, Jump{int(le.Uint16(e)), int(le.Uint16(e[2:])), int(le.Uint16(e[4:])), int(le.Uint16(e[6:]))})
			}
		case "JUM2":
			for i := 0; i < cnt && i*11+11 <= len(body); i++ {
				e := body[i*11:]
				m.Lands = append(m.Lands, Land{int(le.Uint16(e)), int(le.Uint16(e[2:])), int(le.Uint16(e[4:])), int(e[10])})
			}
		}
		p += ln
	}
	return m, nil
}

// Walkable 地砖坐标 (x,y) 是否可走 (启发式, 未启用)。
func (m *Map) Walkable(svc *WalkableService, x, y int) bool {
	if x < 0 || y < 0 || x >= m.Size || y >= m.Rows {
		return false
	}
	i := y*m.Size + x
	if m.Objs[i] != 0 {
		return false
	}
	return gidWalkable(m.Tiles[i], svc)
}

func gidWalkable(gid uint16, svc *WalkableService) bool {
	switch {
	case gid == 0:
		return false
	case isWater(gid):
		return false
	}
	if svc != nil && svc.Strict {
		return false
	}
	return true
}

func isWater(gid uint16) bool {
	switch {
	case gid >= 180 && gid <= 182:
		return true
	case gid >= 1754 && gid <= 1790:
		return true
	case gid >= 5552 && gid <= 5579:
		return true
	}
	return false
}

// LoadLinks 只读头和分段 (跳过地砖表), 给传送表用: 返回地图号, 出入口格, 落点。
func LoadLinks(path string) (id int, jumps []Jump, lands []Land, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, nil, err
	}
	defer f.Close()
	h := make([]byte, 40)
	if _, err = f.ReadAt(h, 0); err != nil || string(h[:4]) != "MAP\x00" {
		return 0, nil, nil, errors.New("mapdata: bad header")
	}
	off := int64(le.Uint32(h[4:]))
	st, err := f.Stat()
	if err != nil || off < 40 || off > st.Size() {
		return 0, nil, nil, errors.New("mapdata: bad section offset")
	}
	tail := make([]byte, st.Size()-off)
	if _, err = f.ReadAt(tail, off); err != nil {
		return 0, nil, nil, err
	}
	// 拼一个只有头的假文件: 地砖表长度置 0
	fake := append(append([]byte{}, h...), tail...)
	le.PutUint32(fake[4:], 40)
	le.PutUint16(fake[0xc:], 0)
	le.PutUint16(fake[0xe:], 0)
	m, err := Parse(fake)
	if err != nil {
		return 0, nil, nil, err
	}
	return m.ID, m.Jumps, m.Lands, nil
}

// LoadFlags 读地砖表里每个子格的过渡标记: 地砖 u32 低 16 位的低 4 位, 位号 = (y&1)*2 + (x&1)。
// 这些标记很少 (地图 2: 581/715000 地砖), 出现在出口一带 —— 客户端走进标记带就停下等服务器换图。
// 返回子格宽高和标记子格集合 (只含置位的)。
func LoadFlags(path string) (w, h int, flagged map[[2]int]bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, nil, err
	}
	defer f.Close()
	hd := make([]byte, 40)
	if _, err = f.ReadAt(hd, 0); err != nil || string(hd[:4]) != "MAP\x00" {
		return 0, 0, nil, errors.New("mapdata: bad header")
	}
	w, h = int(le.Uint16(hd[0xc:])), int(le.Uint16(hd[0xe:]))
	tw, th := w/2, h/2
	buf := make([]byte, tw*th*4)
	if _, err = f.ReadAt(buf, 40); err != nil {
		return 0, 0, nil, err
	}
	flagged = map[[2]int]bool{}
	for i := 0; i < tw*th; i++ {
		b := buf[i*4] & 0xf // 低 16 位的低字节
		if b == 0 {
			continue
		}
		tx, ty := i%tw, i/tw
		for bit := 0; bit < 4; bit++ {
			if b&(1<<bit) != 0 {
				flagged[[2]int{tx*2 + bit&1, ty*2 + bit>>1}] = true
			}
		}
	}
	return w, h, flagged, nil
}
