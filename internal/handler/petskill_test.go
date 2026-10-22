package handler

import "testing"

func TestPetSkillParse(t *testing.T) {
	k := parsePetSkill("40,一击必杀,说明,Mighty,倍2 回避 30,0")
	if k == nil || k.mul != 2 || k.evade != 30 || k.fn != "Mighty" {
		t.Fatalf("mighty %+v", k)
	}
	h := parsePetSkill("661,水灵滋润,说明,Heal,HP 30,10")
	if h == nil || h.heal != 30 || h.fp != 10 {
		t.Fatalf("heal %+v", h)
	}
	if parsePetSkill("# x") != nil {
		t.Fatal("comment")
	}
}
