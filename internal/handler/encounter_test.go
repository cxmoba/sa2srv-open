package handler

import "testing"

func TestEncounterSafeMaps(t *testing.T) {
	old := safeMaps
	defer func() { safeMaps = old }()
	safeMaps = map[int]bool{100: true}
	if encounterMap(100) {
		t.Fatal("safe map 100 encounters")
	}
	if Data != nil && len(Data.Encount[1]) > 0 && !encounterMap(1) {
		t.Fatal("field map 1 should encounter")
	}
	s := &session{inBattle: true}
	if stepEncounter(nil, s, 1, 50) {
		t.Fatal("encounter while in battle")
	}
}
