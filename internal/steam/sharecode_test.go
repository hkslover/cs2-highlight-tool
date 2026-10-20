package steam

import (
	"errors"
	"testing"
)

// Real CS2/CS:GO share codes. The expected values were cross-checked against the
// reference implementation (csgo-sharecode) and against the Game Coordinator
// payloads those codes belong to: the match ID and reservation ID are the
// payload's match ID and final round reservation ID, and the TV port is the low
// 16 bits of the payload's tv_port field (the wire format only has 2 bytes for
// it, so modern 32-bit TV port values are truncated by design).
func TestParseShareCodeRealVectors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want ShareCode
	}{
		{
			name: "cs2 competitive match",
			raw:  "CSGO-3MD4K-ShNFS-p3t6a-dxQJH-f9hWA",
			want: ShareCode{MatchID: 3668142292599833094, ReservationID: 3668148000611369297, TVPort: 20625},
		},
		{
			name: "cs2 competitive match with shuffled slots",
			raw:  "CSGO-98ykZ-EJ952-QzetT-kWNp6-Oe2ED",
			want: ShareCode{MatchID: 3668529891923460401, ReservationID: 3668537425296097309, TVPort: 63752},
		},
		{
			name: "csgo era match",
			raw:  "CSGO-ertoU-6E6wT-Qamy2-Xivk6-wyTaK",
			want: ShareCode{MatchID: 3498486364238774422, ReservationID: 3498492722937856210, TVPort: 6168},
		},
		{
			name: "tournament match with overtime",
			raw:  "CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6SA",
			want: ShareCode{MatchID: 3189266349767524357, ReservationID: 3189280594026561777, TVPort: 27214},
		},
		{
			name: "legacy payload without demo link",
			raw:  "CSGO-7AJey-YsQmH-vireS-w7mOm-OijDA",
			want: ShareCode{MatchID: 3189864986604208129, ReservationID: 3189870870709403769, TVPort: 37988},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseShareCode(test.raw)
			if err != nil {
				t.Fatalf("ParseShareCode(%q) returned error: %v", test.raw, err)
			}
			if got != test.want {
				t.Fatalf("ParseShareCode(%q) = %+v, want %+v", test.raw, got, test.want)
			}
			// Encoding must reproduce Valve's canonical spelling, which also
			// proves the digit order (little-endian) is right.
			if encoded := got.String(); encoded != test.raw {
				t.Fatalf("ShareCode.String() = %q, want %q", encoded, test.raw)
			}
		})
	}
}

func TestParseShareCodeAcceptsLooseInput(t *testing.T) {
	want := ShareCode{MatchID: 3189266349767524357, ReservationID: 3189280594026561777, TVPort: 27214}
	for _, raw := range []string{
		"CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6SA",
		"csgo-9NCRf-oJYL5-BKKeO-rptnD-My6SA",
		"CSGO9NCRfoJYL5BKKeOrptnDMy6SA",
		"  9NCRf-oJYL5-BKKeO-rptnD-My6SA  ",
	} {
		got, err := ParseShareCode(raw)
		if err != nil {
			t.Fatalf("ParseShareCode(%q) returned error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseShareCode(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

// The payload case is significant: 'l' and 'I' are absent from the dictionary,
// so lowering the case of a valid code would make it undecodable.
func TestParseShareCodeIsCaseSensitive(t *testing.T) {
	lowered := "csgo-9ncrf-ojyl5-bkkeo-rptnd-my6sa"
	if _, err := ParseShareCode(lowered); !errors.Is(err, ErrInvalidShareCode) {
		t.Fatalf("ParseShareCode(%q) error = %v, want ErrInvalidShareCode", lowered, err)
	}
}

func TestParseShareCodeRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"CSGO-12345",
		"CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6S",   // 24 characters
		"CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6SA5", // 26 characters
		"CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6S0",  // 0 is not in the dictionary
		"CSGO-9NCRf-oJYL5-BKKeO-rptnD-My6SI",  // I is not in the dictionary
		"76561198000000000",
	} {
		parsed, err := ParseShareCode(raw)
		if !errors.Is(err, ErrInvalidShareCode) {
			t.Fatalf("ParseShareCode(%q) = %+v, error = %v, want ErrInvalidShareCode", raw, parsed, err)
		}
	}
}

func TestShareCodeRoundTrip(t *testing.T) {
	for _, original := range []ShareCode{
		{MatchID: 1},
		{MatchID: ^uint64(0), ReservationID: ^uint64(0), TVPort: ^uint16(0)},
		{MatchID: 3668142292599833094, ReservationID: 3668148000611369297, TVPort: 20625},
	} {
		decoded, err := ParseShareCode(original.String())
		if err != nil {
			t.Fatalf("round trip of %+v returned error: %v", original, err)
		}
		if decoded != original {
			t.Fatalf("round trip = %+v, want %+v", decoded, original)
		}
	}
}

func TestSteamIDConversion(t *testing.T) {
	const accountID = 1187846741
	const steamID = uint64(76561199148112469)
	if got := SteamID64FromAccountID(accountID); got != steamID {
		t.Errorf("SteamID64FromAccountID(%d) = %d, want %d", accountID, got, steamID)
	}
	if got := AccountIDFromSteamID64(steamID); got != accountID {
		t.Errorf("AccountIDFromSteamID64(%d) = %d, want %d", steamID, got, accountID)
	}
}
