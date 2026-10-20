//go:build windows

package steam

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is the only place that touches native Steam code. The local
// steam_api64.dll is loaded from the user's own Counter-Strike installation and
// every used export is validated before it is called, so a missing or ancient
// library is reported as ClientAPIUnavailable instead of crashing.

const (
	// ESteamAPIInitResult values returned by SteamAPI_InitFlat.
	eSteamAPIInitResultOK              = 0
	eSteamAPIInitResultFailedGeneric   = 1
	eSteamAPIInitResultNoSteamClient   = 2
	eSteamAPIInitResultVersionMismatch = 3
	// kCchMaxSteamErrMsg is the size of the SteamErrMsg buffer SteamAPI_InitFlat
	// writes its error message into.
	kCchMaxSteamErrMsg = 1024
)

// steamUserInterfaceVersions are probed in order: the first version the local
// library recognises wins. Newer SDKs bump the version suffix, so the newest
// known value comes first and older local libraries still work.
var steamUserInterfaceVersions = []string{
	"SteamUser023",
	"SteamUser022",
	"SteamUser021",
	"SteamUser020",
	"SteamUser019",
	"SteamUser018",
}

// steamAPILoaded caches the successfully bound library for the process
// lifetime; failures are not cached so a corrected CS2 path can be retried.
var (
	steamAPILoadMu sync.Mutex
	steamAPILoaded *steamAPIModule
)

// steamAPIModule owns one loaded steam_api64.dll and its bound exports.
type steamAPIModule struct {
	handle windows.Handle
	path   string

	initFlat       uintptr
	shutdown       uintptr
	runCallbacks   uintptr
	isSteamRunning uintptr
	getHSteamUser  uintptr
	getHSteamPipe  uintptr
	findOrCreate   uintptr
	userSteamID    uintptr
	userBLoggedOn  uintptr
}

// loadSteamAPI loads and binds the first usable local steam_api64.dll. A
// successfully loaded module is cached for the process lifetime; LoadLibrary
// reference counting keeps repeated imports cheap.
func loadSteamAPI(dirs []string) (*steamAPIModule, error) {
	steamAPILoadMu.Lock()
	defer steamAPILoadMu.Unlock()

	if steamAPILoaded != nil {
		return steamAPILoaded, nil
	}

	candidates := steamAPICandidatePaths(dirs)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: 已在 CS2 安装目录中查找 %s", ErrSteamAPINotFound, steamAPIFileName)
	}

	var lastErr error
	for _, candidate := range candidates {
		module, err := bindSteamAPI(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		steamAPILoaded = module
		return module, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrSteamAPINotFound, lastErr)
}

func bindSteamAPI(path string) (*steamAPIModule, error) {
	// LOAD_WITH_ALTERED_SEARCH_PATH makes the library resolve its own
	// dependencies (steamclient64.dll and friends) relative to its own
	// directory, which is how a game-installed copy expects to be loaded.
	handle, err := windows.LoadLibraryEx(path, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return nil, fmt.Errorf("%s: 加载失败: %w", path, err)
	}

	module := &steamAPIModule{handle: handle, path: path}
	for _, name := range requiredSteamAPIExports {
		address, err := windows.GetProcAddress(handle, name)
		if err != nil {
			_ = windows.FreeLibrary(handle)
			return nil, fmt.Errorf("%s: 缺少导出 %s（Steamworks SDK 版本过旧）", path, name)
		}
		if err := module.assign(name, address); err != nil {
			_ = windows.FreeLibrary(handle)
			return nil, err
		}
	}
	return module, nil
}

// assign keeps the required export list and the bound function pointers in sync:
// an export listed as required without a binding fails here instead of silently
// calling a zero address.
func (m *steamAPIModule) assign(name string, address uintptr) error {
	switch name {
	case "SteamAPI_InitFlat":
		m.initFlat = address
	case "SteamAPI_Shutdown":
		m.shutdown = address
	case "SteamAPI_RunCallbacks":
		m.runCallbacks = address
	case "SteamAPI_IsSteamRunning":
		m.isSteamRunning = address
	case "SteamAPI_GetHSteamUser":
		m.getHSteamUser = address
	case "SteamAPI_GetHSteamPipe":
		m.getHSteamPipe = address
	case "SteamInternal_FindOrCreateUserInterface":
		m.findOrCreate = address
	case "SteamAPI_ISteamUser_GetSteamID":
		m.userSteamID = address
	case "SteamAPI_ISteamUser_BLoggedOn":
		m.userBLoggedOn = address
	default:
		return fmt.Errorf("未绑定的必需导出: %s", name)
	}
	return nil
}

// InitFlat calls SteamAPI_InitFlat. The returned message is the library's own
// diagnostic text and is empty on success.
func (m *steamAPIModule) InitFlat() (int32, string) {
	var errMsg [kCchMaxSteamErrMsg]byte
	result, _, _ := syscall.SyscallN(m.initFlat, uintptr(unsafe.Pointer(&errMsg[0])))
	return int32(result), goStringFromBytes(errMsg[:])
}

func (m *steamAPIModule) Shutdown() {
	if m.shutdown == 0 {
		return
	}
	_, _, _ = syscall.SyscallN(m.shutdown)
}

func (m *steamAPIModule) RunCallbacks() {
	if m.runCallbacks == 0 {
		return
	}
	_, _, _ = syscall.SyscallN(m.runCallbacks)
}

func (m *steamAPIModule) IsSteamRunning() bool {
	result, _, _ := syscall.SyscallN(m.isSteamRunning)
	return result&0xFF != 0
}

func (m *steamAPIModule) HSteamUser() uintptr {
	result, _, _ := syscall.SyscallN(m.getHSteamUser)
	return result
}

func (m *steamAPIModule) HSteamPipe() uintptr {
	result, _, _ := syscall.SyscallN(m.getHSteamPipe)
	return result
}

// FindUserInterface returns the ISteamUser interface for the first version the
// library recognises, or 0 when none matched.
func (m *steamAPIModule) FindUserInterface(hSteamUser uintptr) uintptr {
	for _, version := range steamUserInterfaceVersions {
		if iface := m.findOrCreateInterface(hSteamUser, version); iface != 0 {
			return iface
		}
	}
	return 0
}

func (m *steamAPIModule) FindOrCreateUserInterface(hSteamUser uintptr, version string) uintptr {
	return m.findOrCreateInterface(hSteamUser, version)
}

func (m *steamAPIModule) findOrCreateInterface(hSteamUser uintptr, version string) uintptr {
	name, err := syscall.BytePtrFromString(version)
	if err != nil {
		return 0
	}
	result, _, _ := syscall.SyscallN(m.findOrCreate, hSteamUser, uintptr(unsafe.Pointer(name)))
	return result
}

func (m *steamAPIModule) UserBLoggedOn(user uintptr) bool {
	if user == 0 {
		return false
	}
	result, _, _ := syscall.SyscallN(m.userBLoggedOn, user)
	return result&0xFF != 0
}

func (m *steamAPIModule) UserSteamID(user uintptr) uint64 {
	if user == 0 {
		return 0
	}
	result, _, _ := syscall.SyscallN(m.userSteamID, user)
	return uint64(result)
}

// callVirtual invokes one C++ virtual method of an interface pointer. The
// interface starts with a pointer to its vtable, so slot index N is stored at
// vtable + N*sizeof(uintptr). This is the only way to reach
// ISteamGameCoordinator: the Steamworks flat API has no accessor for it.
func callVirtual(iface uintptr, slot int) (uintptr, error) {
	method, err := vtableMethod(iface, slot)
	if err != nil {
		return 0, err
	}
	result, _, _ := syscall.SyscallN(method)
	return result, nil
}

func callVirtualWithArgs(iface uintptr, slot int, args ...uintptr) (uintptr, error) {
	method, err := vtableMethod(iface, slot)
	if err != nil {
		return 0, err
	}
	result, _, _ := syscall.SyscallN(method, append([]uintptr{iface}, args...)...)
	return result, nil
}

// vtableMethod resolves the function pointer of one virtual method.
func vtableMethod(iface uintptr, slot int) (uintptr, error) {
	if iface == 0 {
		return 0, fmt.Errorf("接口未初始化")
	}
	vtable, err := readForeignPointer(iface)
	if err != nil {
		return 0, err
	}
	if vtable == 0 {
		return 0, fmt.Errorf("接口虚表为空")
	}
	method, err := readForeignPointer(vtable + uintptr(slot)*pointerSize)
	if err != nil {
		return 0, err
	}
	if method == 0 {
		return 0, fmt.Errorf("接口方法 %d 为空", slot)
	}
	return method, nil
}

// pointerSize is one pointer-sized word; the Steam interfaces are only ever
// used on 64-bit Windows builds.
var pointerSize = unsafe.Sizeof(uintptr(0))

// readForeignPointer reads one pointer-sized word from an address inside this
// process. Interface and vtable addresses come from the native Steam library as
// plain integers, so they are read through the documented Win32 API rather than
// converted into an unsafe pointer: a raw integer cannot be proven to point at
// live memory, while ReadProcessMemory validates the address and fails safely.
func readForeignPointer(address uintptr) (uintptr, error) {
	if address == 0 {
		return 0, fmt.Errorf("接口地址为空")
	}
	buffer := make([]byte, int(pointerSize))
	var read uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), address, &buffer[0], pointerSize, &read); err != nil {
		return 0, fmt.Errorf("读取接口内存失败(0x%X): %w", address, err)
	}
	if read != pointerSize {
		return 0, fmt.Errorf("读取接口内存不完整(0x%X): %d 字节", address, read)
	}
	var value uintptr
	for index := len(buffer) - 1; index >= 0; index-- {
		value = value<<8 | uintptr(buffer[index])
	}
	return value, nil
}

// goStringFromBytes converts a NUL terminated byte buffer returned by the Steam
// library into a Go string.
func goStringFromBytes(buffer []byte) string {
	for index, value := range buffer {
		if value == 0 {
			return string(buffer[:index])
		}
	}
	return string(buffer)
}
