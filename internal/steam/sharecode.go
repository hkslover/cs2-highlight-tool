package steam

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// shareCodeDictionary is Valve's base-57 alphabet used by CS2/CS:GO match share
// codes (0/O/I/l are excluded to avoid ambiguous duplicates).
const shareCodeDictionary = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789"

const (
	shareCodePrefix = "CSGO"
	// shareCodeChars is the number of dictionary characters after the prefix.
	shareCodeChars = 25
	// shareCodeBytes is the payload size in bytes: match ID (8) + reservation ID
	// (8) + TV port (2), each little-endian.
	shareCodeBytes = 18
)

// ErrInvalidShareCode reports a share code that is not a CS2 match share code.
var ErrInvalidShareCode = errors.New("无效的比赛分享码")

// ShareCode is the decoded content of a CS2 match share code. These three
// values are exactly the arguments the Game Coordinator expects for a specific
// match lookup, so a share code is enough to resolve a demo download URL.
type ShareCode struct {
	MatchID       uint64
	ReservationID uint64
	TVPort        uint16
}

// ParseShareCode decodes a CS2 match share code. Surrounding spaces, the
// "CSGO" prefix (in any case) and the dash separators are optional, but the
// payload characters must keep their original case because Valve's dictionary
// is case sensitive.
func ParseShareCode(raw string) (ShareCode, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return ShareCode{}, fmt.Errorf("%w: 内容为空", ErrInvalidShareCode)
	}
	// Only the literal prefix may differ in case: the 57 character dictionary
	// holds both 'F' and 'f' but only 'L' (no 'l') and only 'i' (no 'I'), so
	// folding the payload case would make distinct codes collide or become
	// undecodable.
	if len(normalized) >= len(shareCodePrefix) && strings.EqualFold(normalized[:len(shareCodePrefix)], shareCodePrefix) {
		normalized = normalized[len(shareCodePrefix):]
	}
	normalized = strings.ReplaceAll(normalized, "-", "")
	if len(normalized) != shareCodeChars {
		return ShareCode{}, fmt.Errorf("%w: 需要 %d 个字符，实际 %d 个", ErrInvalidShareCode, shareCodeChars, len(normalized))
	}

	// Share code digits are little-endian: the first character is the least
	// significant base-57 digit, the last one the most significant.
	total := new(big.Int)
	base := big.NewInt(int64(len(shareCodeDictionary)))
	for index := len(normalized) - 1; index >= 0; index-- {
		digit := strings.IndexByte(shareCodeDictionary, normalized[index])
		if digit < 0 {
			return ShareCode{}, fmt.Errorf("%w: 含非法字符 %q", ErrInvalidShareCode, normalized[index])
		}
		total.Mul(total, base)
		total.Add(total, big.NewInt(int64(digit)))
	}
	if total.BitLen() > shareCodeBytes*8 {
		return ShareCode{}, fmt.Errorf("%w: 数值超出范围", ErrInvalidShareCode)
	}

	payload := make([]byte, shareCodeBytes)
	total.FillBytes(payload)
	return ShareCode{
		MatchID:       littleEndianUint64(payload[0:8]),
		ReservationID: littleEndianUint64(payload[8:16]),
		TVPort:        uint16(littleEndianUint64(payload[16:18])),
	}, nil
}

// String encodes the share code back into Valve's canonical
// "CSGO-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX" form.
func (sc ShareCode) String() string {
	match := littleEndianBytes(sc.MatchID, 8)
	reservation := littleEndianBytes(sc.ReservationID, 8)
	port := littleEndianBytes(uint64(sc.TVPort), 2)
	payload := make([]byte, 0, shareCodeBytes)
	payload = append(payload, match...)
	payload = append(payload, reservation...)
	payload = append(payload, port...)

	total := new(big.Int).SetBytes(payload)
	base := big.NewInt(int64(len(shareCodeDictionary)))
	chars := make([]byte, shareCodeChars)
	for index := 0; index < shareCodeChars; index++ {
		quotient, remainder := new(big.Int).QuoRem(total, base, new(big.Int))
		chars[index] = shareCodeDictionary[remainder.Int64()]
		total = quotient
	}

	var builder strings.Builder
	builder.WriteString(shareCodePrefix)
	for start := 0; start < shareCodeChars; start += 5 {
		builder.WriteByte('-')
		builder.Write(chars[start : start+5])
	}
	return builder.String()
}

func littleEndianBytes(value uint64, size int) []byte {
	out := make([]byte, size)
	for index := 0; index < size; index++ {
		out[index] = byte(value >> (8 * index))
	}
	return out
}

func littleEndianUint64(raw []byte) uint64 {
	var value uint64
	for index := len(raw) - 1; index >= 0; index-- {
		value = value<<8 | uint64(raw[index])
	}
	return value
}
