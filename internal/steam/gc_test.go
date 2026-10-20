package steam

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// fakeTransport replays a scripted Game Coordinator conversation: the first
// silentAttempts sends get no reply at all, otherwise the payload arrives after
// latency empty polls.
type fakeTransport struct {
	payload        []byte
	latency        int
	silentAttempts int
	sends          int
	polls          int
	sendErr        error
	lastPayload    []byte
	lastType       uint32
}

func (f *fakeTransport) Send(messageType uint32, payload []byte) error {
	f.sends++
	f.polls = 0
	f.lastType = messageType
	f.lastPayload = payload
	return f.sendErr
}

func (f *fakeTransport) Receive() (uint32, []byte, bool, error) {
	f.polls++
	if f.sends <= f.silentAttempts {
		return 0, nil, false, nil
	}
	if f.polls > f.latency && f.payload != nil {
		payload := f.payload
		f.payload = nil
		return messageMatchList, payload, true, nil
	}
	return 0, nil, false, nil
}

type fakeSession struct {
	transport gcTransport
	steamID   uint64
	closed    bool
}

func (s *fakeSession) Transport() gcTransport { return s.transport }
func (s *fakeSession) SteamID() uint64        { return s.steamID }
func (s *fakeSession) Close()                 { s.closed = true }

func fastOptions() gcRequestOptions {
	return gcRequestOptions{
		attempts:        3,
		retryDelay:      5 * time.Millisecond,
		responseTimeout: 40 * time.Millisecond,
		pollInterval:    time.Millisecond,
	}
}

func TestEncodeDecodeGCMessage(t *testing.T) {
	body := []byte{0x01, 0x02, 0x03}
	framed := encodeGCMessage(messageRequestRecentUserGames, body)
	if len(framed) != gcMessageHeaderSize+len(body) {
		t.Fatalf("framed length = %d, want %d", len(framed), gcMessageHeaderSize+len(body))
	}

	messageType, decoded, ok := decodeGCMessage(framed)
	if !ok {
		t.Fatal("decodeGCMessage rejected a framed message")
	}
	if messageType != messageRequestRecentUserGames {
		t.Errorf("message type = %d, want %d", messageType, messageRequestRecentUserGames)
	}
	if string(decoded) != string(body) {
		t.Errorf("body = %v, want %v", decoded, body)
	}

	// The protobuf flag must be set in the wire header.
	if flag := readUint32LE(framed[0:4]) & gcProtobufFlag; flag == 0 {
		t.Error("protobuf flag is not set")
	}

	if _, _, ok := decodeGCMessage([]byte{1, 2, 3}); ok {
		t.Error("decodeGCMessage accepted a too short message")
	}
	withoutFlag := make([]byte, gcMessageHeaderSize)
	putUint32LE(withoutFlag[0:4], messageMatchList)
	if _, _, ok := decodeGCMessage(withoutFlag); ok {
		t.Error("decodeGCMessage accepted a message without the protobuf flag")
	}
}

func TestRequestMatchListReturnsResponse(t *testing.T) {
	transport := &fakeTransport{payload: []byte("payload"), latency: 2}
	payload, err := requestMatchList(context.Background(), transport, messageRequestRecentUserGames, []byte("req"), fastOptions())
	if err != nil {
		t.Fatalf("requestMatchList returned error: %v", err)
	}
	if string(payload) != "payload" {
		t.Fatalf("payload = %q, want %q", payload, "payload")
	}
	if transport.sends != 1 {
		t.Errorf("sends = %d, want 1", transport.sends)
	}
	if transport.lastType != messageRequestRecentUserGames {
		t.Errorf("message type = %d", transport.lastType)
	}
}

func TestRequestMatchListRetriesSilentCoordinator(t *testing.T) {
	transport := &fakeTransport{payload: []byte("payload"), silentAttempts: 2}
	payload, err := requestMatchList(context.Background(), transport, messageRequestRecentUserGames, []byte("req"), fastOptions())
	if err != nil {
		t.Fatalf("requestMatchList returned error: %v", err)
	}
	if string(payload) != "payload" {
		t.Fatalf("payload = %q", payload)
	}
	if transport.sends != 3 {
		t.Errorf("sends = %d, want 3 (two silent attempts then success)", transport.sends)
	}
}

func TestRequestMatchListReportsUnavailableAfterAttempts(t *testing.T) {
	transport := &fakeTransport{payload: []byte("payload"), silentAttempts: 99}
	options := fastOptions()
	_, err := requestMatchList(context.Background(), transport, messageRequestRecentUserGames, []byte("req"), options)
	if !errors.Is(err, ErrGCUnavailable) {
		t.Fatalf("error = %v, want ErrGCUnavailable", err)
	}
	if transport.sends != options.attempts {
		t.Errorf("sends = %d, want %d", transport.sends, options.attempts)
	}
}

func TestRequestMatchListReportsSendFailure(t *testing.T) {
	transport := &fakeTransport{sendErr: errors.New("send failed")}
	_, err := requestMatchList(context.Background(), transport, messageRequestRecentUserGames, []byte("req"), fastOptions())
	if !errors.Is(err, ErrGCUnavailable) {
		t.Fatalf("error = %v, want ErrGCUnavailable", err)
	}
}

func TestRequestMatchListHonorsContextCancellation(t *testing.T) {
	transport := &fakeTransport{payload: []byte("payload"), silentAttempts: 99}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := requestMatchList(ctx, transport, messageRequestRecentUserGames, []byte("req"), fastOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestListRecentMatchesMapsExpectedStatesToStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ClientStatus
	}{
		{"steam api missing", ErrSteamAPINotFound, ClientAPIUnavailable},
		{"steam not running", ErrSteamNotRunning, ClientNotRunning},
		{"steam not logged in", ErrSteamNotLoggedIn, ClientNotLoggedIn},
		{"unsupported platform", ErrUnsupportedOS, ClientUnsupportedPlatform},
		{"coordinator silent", ErrGCUnavailable, ClientGCUnavailable},
		{"unexpected failure", errors.New("boom"), ClientGCUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := listRecentMatches(context.Background(), ListOptions{}, func(context.Context, ListOptions) (gcSession, error) {
				return nil, test.err
			})
			if err != nil {
				t.Fatalf("listRecentMatches returned error: %v", err)
			}
			if result.Status != test.want {
				t.Fatalf("status = %q, want %q", result.Status, test.want)
			}
			if result.Matches == nil {
				t.Error("matches must be an empty slice, not nil")
			}
		})
	}
}

func TestListRecentMatchesUsesLocalAccountAndParsesList(t *testing.T) {
	const steamID = uint64(76561199148112469) // account ID 1187846741
	match := &CDataGCCStrike15V2_MatchInfo{
		Matchid:            proto.Uint64(3668142292599833094),
		Matchtime:          proto.Uint32(1708111862),
		Watchablematchinfo: &WatchableMatchInfo{ServerIp: proto.Uint32(273), TvPort: proto.Uint32(1331187857)},
		Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{
			roundStats(3668148000611369297, "http://replay273.valve.net/730/0036681480006111369297_1331187857.dem.bz2", accountIDs, 4104),
		},
	}
	transport := &fakeTransport{payload: wrapMatchList(t, match)}
	session := &fakeSession{transport: transport, steamID: steamID}

	result, err := listRecentMatches(context.Background(), ListOptions{}, func(context.Context, ListOptions) (gcSession, error) {
		return session, nil
	})
	if err != nil {
		t.Fatalf("listRecentMatches returned error: %v", err)
	}
	if result.Status != ClientReady {
		t.Fatalf("status = %q, want %q", result.Status, ClientReady)
	}
	if result.SteamID != "76561199148112469" {
		t.Errorf("steam id = %q", result.SteamID)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(result.Matches))
	}
	if result.Matches[0].ShareCode != "CSGO-3MD4K-ShNFS-p3t6a-dxQJH-f9hWA" {
		t.Errorf("share code = %q", result.Matches[0].ShareCode)
	}
	if !session.closed {
		t.Error("session was not closed")
	}

	// The request body must carry the local account ID.
	request := &CMsgGCCStrike15V2_MatchListRequestRecentUserGames{}
	if err := proto.Unmarshal(transport.lastPayload, request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.GetAccountid() != 1187846741 {
		t.Errorf("request account id = %d, want 1187846741", request.GetAccountid())
	}
}

func TestFetchMatchResolvesShareCode(t *testing.T) {
	shareCode, err := ParseShareCode("CSGO-3MD4K-ShNFS-p3t6a-dxQJH-f9hWA")
	if err != nil {
		t.Fatalf("ParseShareCode: %v", err)
	}
	match := &CDataGCCStrike15V2_MatchInfo{
		Matchid:            proto.Uint64(shareCode.MatchID),
		Matchtime:          proto.Uint32(1708111862),
		Watchablematchinfo: &WatchableMatchInfo{ServerIp: proto.Uint32(273), TvPort: proto.Uint32(1331187857)},
		Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{
			roundStats(shareCode.ReservationID, "http://replay273.valve.net/730/x.dem.bz2", accountIDs, 4104),
		},
	}
	transport := &fakeTransport{payload: wrapMatchList(t, match)}

	item, err := fetchMatch(context.Background(), ListOptions{}, shareCode, func(context.Context, ListOptions) (gcSession, error) {
		return &fakeSession{transport: transport}, nil
	})
	if err != nil {
		t.Fatalf("fetchMatch returned error: %v", err)
	}
	if item.MatchID != "3668142292599833094" {
		t.Fatalf("match id = %q", item.MatchID)
	}

	// The request body must carry the three share code values.
	request := &CMsgGCCStrike15V2_MatchListRequestFullGameInfo{}
	if err := proto.Unmarshal(transport.lastPayload, request); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if request.GetMatchid() != shareCode.MatchID || request.GetOutcomeid() != shareCode.ReservationID || request.GetToken() != uint32(shareCode.TVPort) {
		t.Errorf("request = %+v, want %+v", request, shareCode)
	}
}

func TestFetchMatchRejectsUnrelatedResponse(t *testing.T) {
	other := &CDataGCCStrike15V2_MatchInfo{
		Matchid:            proto.Uint64(1),
		Watchablematchinfo: &WatchableMatchInfo{},
		Roundstatsall: []*CMsgGCCStrike15V2_MatchmakingServerRoundStats{
			roundStats(2, "", accountIDs, 4104),
		},
	}
	transport := &fakeTransport{payload: wrapMatchList(t, other)}
	_, err := fetchMatch(context.Background(), ListOptions{}, ShareCode{MatchID: 999}, func(context.Context, ListOptions) (gcSession, error) {
		return &fakeSession{transport: transport}, nil
	})
	if err == nil {
		t.Fatal("fetchMatch accepted a response for another match")
	}
}

func TestFetchMatchReportsSessionFailure(t *testing.T) {
	_, err := fetchMatch(context.Background(), ListOptions{}, ShareCode{MatchID: 1}, func(context.Context, ListOptions) (gcSession, error) {
		return nil, ErrSteamNotRunning
	})
	if !errors.Is(err, ErrSteamNotRunning) {
		t.Fatalf("error = %v, want ErrSteamNotRunning", err)
	}
}

func TestSteamAPICandidatePaths(t *testing.T) {
	root := t.TempDir()
	withDLL := filepath.Join(root, "bin", "win64")
	withoutDLL := filepath.Join(root, "empty")
	for _, dir := range []string{withDLL, withoutDLL} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	dllPath := filepath.Join(withDLL, steamAPIFileName)
	if err := os.WriteFile(dllPath, []byte("stub"), 0644); err != nil {
		t.Fatalf("write stub dll: %v", err)
	}

	candidates := steamAPICandidatePaths([]string{withoutDLL, withDLL, withDLL, "  ", ""})
	if len(candidates) != 1 {
		t.Fatalf("candidates = %v, want exactly one entry", candidates)
	}
	if candidates[0] != dllPath {
		t.Errorf("candidate = %q, want %q", candidates[0], dllPath)
	}
	if len(steamAPICandidatePaths(nil)) != 0 {
		t.Error("nil directories must produce no candidates")
	}
}
