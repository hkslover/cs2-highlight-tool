package producegame

import (
	"bytes"
	"testing"
)

// TestPovVPKEmbeddedBytes verifies the embedded asset and the team-aware rules.
// Archive/resource integrity and reproducibility are tested by tools/hud-pov-vpk.
func TestPovVPKEmbeddedBytes(t *testing.T) {
	if len(PovVPK) == 0 {
		t.Fatal("PovVPK is empty; expected embedded asset bytes")
	}
	// Hide the whole container (including its red background) by default.
	// HUD team classes follow the viewed player, so both POV teams must opt in.
	for _, rule := range []string{
		".healthbar-container\n{\n\topacity: 0;\n}",
		".HUD--team--ct #TeamLargeCT .healthbar-container,\n" +
			".HUD--team--terrorist #TeamLargeT .healthbar-container\n{\n\topacity: 1;\n}",
	} {
		if !bytes.Contains(PovVPK, []byte(rule)) {
			t.Fatalf("PovVPK missing team-aware health bar rule: %s", rule)
		}
	}
}
