package saac

import (
	"path/filepath"
	"testing"
)

func TestPets(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 3; i++ {
		if err := s.SavePet(&Pet{CharID: 0x100, Slot: i, Name: "P", Species: "S", Level: 1, HP: 9, MaxHP: 9}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetBattlePet(0x100, 1); err != nil {
		t.Fatal(err)
	}
	ps, err := s.ListPets(0x100)
	if err != nil || len(ps) != 3 {
		t.Fatalf("list %v %v", ps, err)
	}
	if ps[0].Battle || !ps[1].Battle || ps[2].Battle {
		t.Fatalf("battle flags %+v", ps)
	}
	if ps[1].PetID() != 0x20001001 {
		t.Fatalf("id %x", ps[1].PetID())
	}
}
