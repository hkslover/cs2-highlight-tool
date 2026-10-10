package producegame

import "testing"

// TestPovVPKEmbeddedBytes verifies the embedded pov.vpk asset is present and
// matches the expected size shipped under assets/pov.vpk.
func TestPovVPKEmbeddedBytes(t *testing.T) {
	if len(PovVPK) == 0 {
		t.Fatal("PovVPK is empty; expected embedded asset bytes")
	}
	// Asset shipped under internal/producegame/assets/pov.vpk is 64084 bytes
	// (~63 KB). If the asset is intentionally updated, refresh this expectation.
	const expectedSize = 64084
	if len(PovVPK) != expectedSize {
		t.Fatalf("PovVPK size = %d, want %d", len(PovVPK), expectedSize)
	}
}
