package handler

import "testing"

func TestDecodeDelta(t *testing.T) {
	var base [objPosSize]byte
	copy(base[:], objPos(0x100, 100, 151, 152, 4, 30002))
	// 真机抓到的心跳: flags=0 id=0x100 dx=0 dy=0
	p, ok := decodeDelta(base, []byte{0, 0, 1, 0, 0, 0, 0})
	if !ok || p != base {
		t.Fatalf("heartbeat changed pos %x", p)
	}
	// dx=+1 dy=-2, dir=6
	p, ok = decodeDelta(base, []byte{4, 0, 1, 0, 0, 1, 0xfe, 6})
	if !ok || p[6] != 152 || p[8] != 150 || p[0xa] != 6 {
		t.Fatalf("delta %x", p)
	}
	// 绝对坐标
	p, ok = decodeDelta(base, []byte{2, 0, 1, 0, 0, 10, 0, 20, 0})
	if !ok || p[6] != 10 || p[8] != 20 {
		t.Fatalf("abs %x", p)
	}
	if _, ok := decodeDelta(base, []byte{2, 0, 1}); ok {
		t.Fatal("short accepted")
	}
}
