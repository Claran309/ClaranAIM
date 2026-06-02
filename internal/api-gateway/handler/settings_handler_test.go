package handler

import "testing"

func TestNormalizeBrowserSkillPathPreservesUploadedFolder(t *testing.T) {
	got := normalizeBrowserSkillPath(`Skill\references\guide.md`)
	if got != "Skill/references/guide.md" {
		t.Fatalf("normalized path = %q, want uploaded folder preserved", got)
	}

	got = normalizeBrowserSkillPath("Skill/SKILL.md")
	if got != "Skill/SKILL.md" {
		t.Fatalf("normalized entry = %q, want Skill/SKILL.md", got)
	}
}
