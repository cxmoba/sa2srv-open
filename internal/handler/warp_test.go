package handler

import "testing"

func TestWarpParseFind(t *testing.T) {
	w, ok := parseWarp("100,177,158,10006,3,17,1")
	if !ok || w.tm != 10006 || w.tx != 3 || w.ty != 17 || w.r != 1 {
		t.Fatalf("parse %+v %v", w, ok)
	}
	if _, ok := parseWarp("# c"); ok {
		t.Fatal("comment parsed")
	}
	old := warps
	defer func() { warps, warpByMap = old, nil }()
	warps, warpByMap = []warpDef{w}, nil
	if findWarp(100, 176, 159) == nil || findWarp(100, 175, 160) != nil || findWarp(1, 177, 158) != nil {
		t.Fatal("findWarp radius")
	}
}
