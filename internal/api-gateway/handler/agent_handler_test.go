package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGatewaySkillSmokeMarkerFromDirUsesUploadedSkillMarker(t *testing.T) {
	skillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Smoke Skill\n\n4. Include the exact marker: `SKILL_OK_7F3A`.\n"), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	got := gatewaySkillSmokeMarkerFromDir(skillDir)
	if got != "SKILL_OK_7F3A" {
		t.Fatalf("marker = %q, want uploaded Skill marker", got)
	}
}

func TestGatewaySkillSmokeMarkerFromDirFindsNestedLegacySkillPackage(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "global", "1", "Skill", "skill")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("Include exact marker: `SKILL_OK_7F3A`."), 0644); err != nil {
		t.Fatalf("write nested SKILL.md: %v", err)
	}

	got := gatewaySkillSmokeMarkerFromDir(root)
	if got != "SKILL_OK_7F3A" {
		t.Fatalf("marker = %q, want nested uploaded Skill marker", got)
	}
}
