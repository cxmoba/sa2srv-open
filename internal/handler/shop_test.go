package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestShopGoodsDefs(t *testing.T) {
	for _, no := range shopGoods {
		d := itemDefs[no]
		if d == nil || d.price <= 0 || d.loc == 0 {
			t.Fatalf("goods %d: %+v", no, d)
		}
	}
	if sellPrice(itemDefs[20625]) != 1 {
		t.Fatal("apple sell")
	}
}

func TestLoadItemsFile(t *testing.T) {
	old := itemDefs
	defer func() { itemDefs = old }()
	n, err := LoadItems("../../data/items.txt")
	if err != nil || n < 10 {
		t.Fatalf("LoadItems: %d %v", n, err)
	}
	for _, no := range shopGoods {
		if itemDefs[no] == nil {
			t.Fatalf("shop goods %d missing from items.txt", no)
		}
	}
	if d := itemDefs[1]; d.kind != "weapon" || d.loc != 3 || !isEquip(d) || equipSlot(d.loc) != 2 {
		t.Fatalf("axe: %+v", d)
	}
	if d := itemDefs[48038]; d.loc != 0x15 || d.desc == "" {
		t.Fatalf("misc: %+v", d)
	}
}

func TestLoadItemsCNOverride(t *testing.T) {
	old, oldBase := itemDefs, itemBase
	defer func() { itemDefs, itemBase = old, oldBase }()
	// stand-in for a generated items_cn.txt (tools/cn/gen_items_cn.py output format)
	base := "# no\tname\tkind\tval\tloc\tstack\tprice\ticon\tdrop\tdesc\textra\n"
	for no := 3000; no < 11000; no++ {
		base += fmt.Sprintf("%d\tItem %d\tmisc\t0\t0x15\t1\t1\t35000\t0\t\t\n", no, no)
	}
	base += "20042\tCN Meat\tfood\t15\t0x13\t99\t10\t35644\t0\tHP+15\t\n1\tCN Axe\tweapon\t9\t0x3\t1\t130\t35000\t0\tAxe\t43:2\n"
	p := filepath.Join(t.TempDir(), "items_cn.txt")
	if err := os.WriteFile(p, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := LoadItemsCN(p)
	if err != nil || n < 8000 {
		t.Fatalf("LoadItemsCN: %d %v", n, err)
	}
	if d := itemDefs[20042]; d == nil || d.kind != "food" || d.val != 15 || d.icon != 35644 {
		t.Fatalf("20042: %+v", d)
	}
	if d := itemDefs[1]; d == nil || d.kind != "weapon" || d.val != 9 || d.loc != 3 {
		t.Fatalf("1 (cn): %+v", d)
	}
	if _, err := LoadItems("../../data/items.txt"); err != nil {
		t.Fatal(err)
	}
	if d := itemDefs[1]; d.val != 6 { // items.txt 覆盖
		t.Fatalf("1 override: %+v", d)
	}
	if itemDefs[20042] == nil || len(itemDefs) < 8000 {
		t.Fatal("base lost after override")
	}
}
