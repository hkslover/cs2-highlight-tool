package steam

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Game Coordinator message types (k_EMsgGCCStrike15_v2_*).
const (
	messageMatchList                  uint32 = 9139
	messageRequestRecentUserGames     uint32 = 9141
	messageRequestFullGameInfo        uint32 = 9147
	gcProtobufFlag                    uint32 = 1 << 31
	gcMessageHeaderSize                      = 8
	steamAppID                               = "730"
	steamGameCoordinatorInterfaceName        = "SteamGameCoordinator001"
	steamClientInterfaceName                 = "SteamClient023"
)

// Expected (non-fatal) states of the Steam integration. They are reported as
// ClientStatus values rather than errors because the user can act on all of
// them.
var (
	ErrSteamNotRunning  = errors.New("Steam 客户端未运行")
	ErrSteamNotLoggedIn = errors.New("Steam 未登录")
	ErrSteamAPINotFound = errors.New("未找到可用的本机 steam_api64.dll")
	ErrGCUnavailable    = errors.New("Game Coordinator 未响应（同一账号的 CS2 正在运行时会出现）")
	ErrUnsupportedOS    = errors.New("当前系统不支持 Steam 官方比赛导入")
)

// gcRequestOptions bounds one Game Coordinator round trip. The Game
// Coordinator occasionally drops a request, especially right after the
// connection is established, so a request is retried a few times before the
// state is reported as unavailable.
type gcRequestOptions struct {
	attempts        int
	retryDelay      time.Duration
	responseTimeout time.Duration
	pollInterval    time.Duration
}

var defaultGCRequestOptions = gcRequestOptions{
	attempts:        3,
	retryDelay:      1500 * time.Millisecond,
	responseTimeout: 8 * time.Second,
	pollInterval:    50 * time.Millisecond,
}

// EGCResult values returned by the Game Coordinator transport (k_EGCResult*).
const (
	eGCResultOK             = 0
	eGCResultNoMessage      = 1
	eGCResultBufferTooSmall = 2
)

// ISteamGameCoordinator001 virtual method slots. The interface exposes no flat
// Steamworks export, so the methods are reached through the vtable.
const (
	gcVtableSendMessage        = 0
	gcVtableIsMessageAvailable = 1
	gcVtableRetrieveMessage    = 2
)

// Session timing. SteamAPI_Init and SteamAPI_RunCallbacks share one OS thread
// (see the platform binding), and the callback pump has to keep running while a
// request is in flight for the pipe to deliver messages.
const (
	steamCallbackInterval     = 20 * time.Millisecond
	steamLoggedOnTimeout      = 3 * time.Second
	steamLoggedOnPollInterval = 100 * time.Millisecond
)

// gCExpectedErrors are the user-actionable states; everything else returned by
// the platform layer is reported as an unavailable Game Coordinator.
var expectedStates = []error{ErrSteamAPINotFound, ErrSteamNotRunning, ErrSteamNotLoggedIn, ErrUnsupportedOS}

// gcTransport is the minimal Game Coordinator transport. It is implemented by
// the platform binding and replaced by a fake in tests.
type gcTransport interface {
	// Send frames and sends one protobuf message body.
	Send(messageType uint32, payload []byte) error
	// Receive returns the next message available, if any. It returns
	// (0, nil, false, nil) when the queue is empty.
	Receive() (messageType uint32, payload []byte, ok bool, err error)
}

// gcSession is one open Steam API session with the Game Coordinator interface.
type gcSession interface {
	Transport() gcTransport
	SteamID() uint64
	Close()
}

// gcSessionFactory opens a session; tests inject a fake implementation.
type gcSessionFactory func(ctx context.Context, options ListOptions) (gcSession, error)

// ListOptions selects the local Steam API library to use.
type ListOptions struct {
	// SteamAPIDirs lists directories searched for a local steam_api64.dll, most
	// preferred first. In practice the caller passes the CS2 installation
	// directory: the DLL ships next to cs2.exe there, so no Valve binary has to
	// be bundled or extracted by this tool.
	SteamAPIDirs []string
}

// ListRecentMatches fetches the recent official matches of the signed-in
// account. Expected user-facing states are returned as ClientStatus values;
// only unexpected failures return an error.
func ListRecentMatches(ctx context.Context, options ListOptions) (*MatchListResult, error) {
	return listRecentMatches(ctx, options, newGCSession)
}

func listRecentMatches(ctx context.Context, options ListOptions, open gcSessionFactory) (*MatchListResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	session, err := open(ctx, options)
	if err != nil {
		return statusResultForError(err), nil
	}
	defer session.Close()

	body, err := marshalRecentGamesRequest(AccountIDFromSteamID64(session.SteamID()))
	if err != nil {
		return nil, fmt.Errorf("构建比赛列表请求失败: %w", err)
	}

	response, err := requestMatchList(ctx, session.Transport(), messageRequestRecentUserGames, body, defaultGCRequestOptions)
	if err != nil {
		return statusResultForError(err), nil
	}

	matches, err := ParseMatchList(response, session.SteamID())
	if err != nil {
		return nil, err
	}
	return &MatchListResult{
		Status:  ClientReady,
		SteamID: fmt.Sprintf("%d", session.SteamID()),
		Matches: matches,
	}, nil
}

// FetchMatch resolves a single match from a share code, which is the only way
// to reach matches older than the recent list. The Game Coordinator's
// FullGameInfo request carries the three values a share code encodes.
func FetchMatch(ctx context.Context, options ListOptions, shareCode ShareCode) (*MatchItem, error) {
	return fetchMatch(ctx, options, shareCode, newGCSession)
}

func fetchMatch(ctx context.Context, options ListOptions, shareCode ShareCode, open gcSessionFactory) (*MatchItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	session, err := open(ctx, options)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	body, err := marshalFullGameInfoRequest(shareCode)
	if err != nil {
		return nil, fmt.Errorf("构建比赛查询请求失败: %w", err)
	}
	response, err := requestMatchList(ctx, session.Transport(), messageRequestFullGameInfo, body, defaultGCRequestOptions)
	if err != nil {
		return nil, err
	}

	matches, err := ParseMatchList(response, session.SteamID())
	if err != nil {
		return nil, err
	}
	for index := range matches {
		if matches[index].MatchID == fmt.Sprintf("%d", shareCode.MatchID) {
			return &matches[index], nil
		}
	}
	return nil, fmt.Errorf("Game Coordinator 未返回该分享码对应的比赛")
}

// statusResultForError maps an expected integration state to a status result.
// Anything unexpected is reported as an unavailable Game Coordinator so the
// frontend always gets an actionable state instead of a raw failure.
func statusResultForError(err error) *MatchListResult {
	result := &MatchListResult{Matches: make([]MatchItem, 0)}
	switch {
	case errors.Is(err, ErrSteamAPINotFound):
		result.Status = ClientAPIUnavailable
	case errors.Is(err, ErrSteamNotRunning):
		result.Status = ClientNotRunning
	case errors.Is(err, ErrSteamNotLoggedIn):
		result.Status = ClientNotLoggedIn
	case errors.Is(err, ErrUnsupportedOS):
		result.Status = ClientUnsupportedPlatform
	default:
		result.Status = ClientGCUnavailable
	}
	return result
}

// requestMatchList sends one request and waits for the match list response,
// retrying a bounded number of times because the Game Coordinator drops
// requests while the connection is still settling.
func requestMatchList(
	ctx context.Context,
	transport gcTransport,
	messageType uint32,
	body []byte,
	options gcRequestOptions,
) ([]byte, error) {
	if transport == nil {
		return nil, fmt.Errorf("Game Coordinator 传输未初始化")
	}
	if options.attempts < 1 {
		options.attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= options.attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := transport.Send(messageType, body); err != nil {
			lastErr = err
		} else {
			payload, err := waitForMatchList(ctx, transport, options)
			if err == nil {
				return payload, nil
			}
			lastErr = err
		}
		if attempt < options.attempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(options.retryDelay):
			}
		}
	}
	if lastErr == nil {
		lastErr = ErrGCUnavailable
	}
	if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w（已重试 %d 次）: %v", ErrGCUnavailable, options.attempts, lastErr)
}

func waitForMatchList(ctx context.Context, transport gcTransport, options gcRequestOptions) ([]byte, error) {
	deadline := time.Now().Add(options.responseTimeout)
	interval := options.pollInterval
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		messageType, payload, ok, err := transport.Receive()
		if err != nil {
			return nil, err
		}
		if ok && messageType == messageMatchList {
			return payload, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrGCUnavailable
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// encodeGCMessage wraps a protobuf body in the 8 byte Game Coordinator header:
// the message type with the protobuf flag set followed by a zero header
// version. The reverse operation is decodeGCMessage.
func encodeGCMessage(messageType uint32, body []byte) []byte {
	framed := make([]byte, gcMessageHeaderSize+len(body))
	putUint32LE(framed[0:4], messageType|gcProtobufFlag)
	putUint32LE(framed[4:8], 0)
	copy(framed[gcMessageHeaderSize:], body)
	return framed
}

// decodeGCMessage strips the Game Coordinator header and reports the inner
// message type. Payloads without the protobuf flag or too short to hold a
// header are reported as not decodable.
func decodeGCMessage(raw []byte) (uint32, []byte, bool) {
	if len(raw) < gcMessageHeaderSize {
		return 0, nil, false
	}
	messageType := readUint32LE(raw[0:4])
	if messageType&gcProtobufFlag == 0 {
		return 0, nil, false
	}
	return messageType &^ gcProtobufFlag, raw[gcMessageHeaderSize:], true
}

func putUint32LE(target []byte, value uint32) {
	target[0] = byte(value)
	target[1] = byte(value >> 8)
	target[2] = byte(value >> 16)
	target[3] = byte(value >> 24)
}

func readUint32LE(raw []byte) uint32 {
	return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
}
