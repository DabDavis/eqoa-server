package database

import "testing"

// Target member (entity/petmemberheal.go) is a heal's alone.
func TestTargetMemberIsAHeals(t *testing.T) {
	if err := checkPetAbility(1, TemplateAbility{Kind: AbilityHeal, Target: AbilityOnMember}); err != nil {
		t.Errorf("a member heal: %v", err)
	}
	if err := checkPetAbility(1, TemplateAbility{Kind: AbilityBuff, Target: AbilityOnMember}); err == nil {
		t.Error("a member buff was taken")
	}
}
