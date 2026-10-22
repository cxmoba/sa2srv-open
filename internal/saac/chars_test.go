package saac

import (
	"path/filepath"
	"testing"
)

func TestChars(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	c := &Char{Account: "a", Slot: 0, Name: "n", Map: 100, X: 125, Y: 125}
	if err := s.CreateChar(c); err != nil || c.ID < 256 {
		t.Fatalf("%v %d", err, c.ID)
	}
	if err := s.CreateChar(&Char{Account: "a", Slot: 0, Name: "x"}); err == nil {
		t.Fatal("want dup")
	}
	l, _ := s.ListChars("a")
	if len(l) != 1 || l[0].Map != 100 || l[0].X != 125 {
		t.Fatalf("%+v", l)
	}
	if l[0].Level != 0 || l[0].HP != 0 {
		t.Fatalf("new char stats %+v", l[0])
	}
	if err := s.SaveStats(c.ID, 5, 1234, 77); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListChars("a"); l[0].Level != 5 || l[0].Exp != 1234 || l[0].HP != 77 {
		t.Fatalf("stats not saved %+v", l[0])
	}
	s.SavePos(c.ID, 100, 1, 2)
	s.DeleteChar("a", 0)
	if l, _ := s.ListChars("a"); len(l) != 0 {
		t.Fatal("not deleted")
	}
}
