//go:build windows

package steam

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// windowsGCSession owns one Steam API session: the library is initialised, the
// callback pump and the shutdown all run on one thread locked to the goroutine
// that created it, which is what the Steamworks API expects.
type windowsGCSession struct {
	api       *steamAPIModule
	gc        uintptr
	transport gcTransport
	steamID   uint64

	init      chan initOutcome
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type initOutcome struct {
	result  int32
	message string
}

func (s *windowsGCSession) Transport() gcTransport { return s.transport }

func (s *windowsGCSession) SteamID() uint64 { return s.steamID }

func (s *windowsGCSession) Close() {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
}

// newGCSession prepares the Game Coordinator connection. Expected states (Steam
// not running, not logged in, no local library) are returned as sentinel errors
// so the caller can map them to a visible status.
func newGCSession(ctx context.Context, options ListOptions) (gcSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	api, err := loadSteamAPI(options.SteamAPIDirs)
	if err != nil {
		return nil, err
	}
	if !api.IsSteamRunning() {
		return nil, ErrSteamNotRunning
	}

	session := &windowsGCSession{
		api:  api,
		init: make(chan initOutcome, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go session.run()

	outcome := <-session.init
	if outcome.result != eSteamAPIInitResultOK {
		session.Close()
		if outcome.result == eSteamAPIInitResultNoSteamClient {
			return nil, ErrSteamNotRunning
		}
		message := strings.TrimSpace(outcome.message)
		if message == "" {
			message = "未知原因"
		}
		return nil, fmt.Errorf("初始化 Steam API 失败(code=%d): %s", outcome.result, message)
	}

	hSteamUser := api.HSteamUser()
	user := api.FindUserInterface(hSteamUser)
	if user == 0 {
		session.Close()
		return nil, fmt.Errorf("未找到本机 Steam 的 ISteamUser 接口")
	}
	if ok, err := session.waitForLoggedOn(ctx, user); !ok {
		session.Close()
		return nil, err
	}
	steamID := api.UserSteamID(user)
	if steamID == 0 {
		session.Close()
		return nil, ErrSteamNotLoggedIn
	}

	gc := api.FindOrCreateUserInterface(hSteamUser, steamGameCoordinatorInterfaceName)
	if gc == 0 {
		session.Close()
		return nil, fmt.Errorf("未找到 Game Coordinator 接口 %s", steamGameCoordinatorInterfaceName)
	}

	session.steamID = steamID
	session.gc = gc
	session.transport = &windowsGCTransport{api: api, gc: gc}
	return session, nil
}

// run owns the whole Steam API lifecycle on one locked OS thread: init, callback
// pump, shutdown.
func (s *windowsGCSession) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	restoreEnv := setSteamAppIDEnvironment()
	result, message := s.api.InitFlat()
	restoreEnv()
	s.init <- initOutcome{result: result, message: message}
	if result != eSteamAPIInitResultOK {
		close(s.done)
		return
	}

	ticker := time.NewTicker(steamCallbackInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			s.api.Shutdown()
			close(s.done)
			return
		case <-ticker.C:
			s.api.RunCallbacks()
		}
	}
}

// waitForLoggedOn polls BLoggedOn while the pump delivers the sign-in
// callbacks, because the flag is not necessarily set the instant after init.
func (s *windowsGCSession) waitForLoggedOn(ctx context.Context, user uintptr) (bool, error) {
	deadline := time.Now().Add(steamLoggedOnTimeout)
	for {
		if s.api.UserBLoggedOn(user) {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, ErrSteamNotLoggedIn
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(steamLoggedOnPollInterval):
		}
	}
}

// setSteamAppIDEnvironment points the Steamworks library at the Counter-Strike
// app session. The library reads the variables during Init, so no
// steam_appid.txt is written anywhere; the previous values are restored right
// after initialisation.
func setSteamAppIDEnvironment() func() {
	previousAppID, hadAppID := os.LookupEnv("SteamAppId")
	previousGameID, hadGameID := os.LookupEnv("SteamGameId")
	_ = os.Setenv("SteamAppId", steamAppID)
	_ = os.Setenv("SteamGameId", steamAppID)
	return func() {
		restoreEnvVar("SteamAppId", previousAppID, hadAppID)
		restoreEnvVar("SteamGameId", previousGameID, hadGameID)
	}
}

func restoreEnvVar(name, value string, hadValue bool) {
	if hadValue {
		_ = os.Setenv(name, value)
		return
	}
	_ = os.Unsetenv(name)
}

// windowsGCTransport frames outgoing protobuf messages and drains the incoming
// queue through the ISteamGameCoordinator vtable.
type windowsGCTransport struct {
	api    *steamAPIModule
	gc     uintptr
	buffer []byte
}

func (t *windowsGCTransport) Send(messageType uint32, payload []byte) error {
	framed := encodeGCMessage(messageType, payload)
	result, err := callVirtualWithArgs(t.gc, gcVtableSendMessage,
		uintptr(messageType|gcProtobufFlag),
		uintptr(unsafe.Pointer(&framed[0])),
		uintptr(len(framed)))
	if err != nil {
		return err
	}
	// The callee keeps no reference to the payload, but the Go garbage
	// collector cannot see the pointer passed through uintptr.
	runtime.KeepAlive(framed)
	if int32(result) != eGCResultOK {
		return fmt.Errorf("发送 Game Coordinator 消息失败: EGCResult=%d", int32(result))
	}
	return nil
}

func (t *windowsGCTransport) Receive() (uint32, []byte, bool, error) {
	size, err := t.availableSize()
	if err != nil {
		return 0, nil, false, err
	}
	if size == 0 {
		return 0, nil, false, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		capacity := int(size) + gcMessageHeaderSize
		if len(t.buffer) < capacity {
			t.buffer = make([]byte, capacity)
		}
		buffer := t.buffer[:capacity]

		var messageType, messageSize uint32
		result, err := callVirtualWithArgs(t.gc, gcVtableRetrieveMessage,
			uintptr(unsafe.Pointer(&messageType)),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
			uintptr(unsafe.Pointer(&messageSize)))
		if err != nil {
			return 0, nil, false, err
		}

		switch int32(result) {
		case eGCResultOK:
		case eGCResultNoMessage:
			return 0, nil, false, nil
		case eGCResultBufferTooSmall:
			// The reported size was stale; grow once and retry. The message
			// stays at the front of the queue.
			size = size*2 + gcMessageHeaderSize
			continue
		default:
			return 0, nil, false, fmt.Errorf("读取 Game Coordinator 消息失败: EGCResult=%d", int32(result))
		}

		if messageSize == 0 || int(messageSize) > len(buffer) {
			return 0, nil, false, fmt.Errorf("Game Coordinator 消息长度异常: %d", messageSize)
		}
		innerType, body, ok := decodeGCMessage(buffer[:messageSize])
		runtime.KeepAlive(buffer)
		if !ok {
			// Non protobuf traffic (for example a hello message) is not
			// interesting here; keep polling.
			continue
		}
		payload := make([]byte, len(body))
		copy(payload, body)
		return innerType, payload, true, nil
	}
	return 0, nil, false, fmt.Errorf("Game Coordinator 消息缓冲区不足")
}

func (t *windowsGCTransport) availableSize() (uint32, error) {
	var size uint32
	result, err := callVirtualWithArgs(t.gc, gcVtableIsMessageAvailable, uintptr(unsafe.Pointer(&size)))
	if err != nil {
		return 0, err
	}
	// A C++ bool is returned in the low byte of the result register.
	if result&0xFF == 0 {
		return 0, nil
	}
	return size, nil
}
