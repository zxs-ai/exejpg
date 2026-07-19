package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// COM 本地服务器 + IExecuteCommand：COM/Player 模型在 Windows 10 下没有多选数量上限。
const comClassID = `{6C1D6A92-8E2B-4B5C-9F17-1D6A3170C8E4}`

const (
	sOK               = uintptr(0)
	eNoInterface      = uintptr(0x80004002)
	ePointer          = uintptr(0x80004003)
	eNotImpl          = uintptr(0x80004001)
	classENoAgg       = uintptr(0x80040110)
	clsctxLocalServer = 0x4
	regclsMultipleUse = 1
	coinitApartment   = 0x2
	wmQuit            = 0x0012
	sigdnFileSysPath  = 0x80058000
)

var (
	ole32                     = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx        = ole32.NewProc("CoInitializeEx")
	procCoUninitialize        = ole32.NewProc("CoUninitialize")
	procCoRegisterClassObject = ole32.NewProc("CoRegisterClassObject")
	procCoRevokeClassObject   = ole32.NewProc("CoRevokeClassObject")
	procCoTaskMemFree         = ole32.NewProc("CoTaskMemFree")

	user32COM              = windows.NewLazySystemDLL("user32.dll")
	procGetMessageW        = user32COM.NewProc("GetMessageW")
	procTranslateMessage   = user32COM.NewProc("TranslateMessage")
	procDispatchMessageW   = user32COM.NewProc("DispatchMessageW")
	procPostThreadMessageW = user32COM.NewProc("PostThreadMessageW")
	kernel32COM            = windows.NewLazySystemDLL("kernel32.dll")
	procGetCurrentThreadID = kernel32COM.NewProc("GetCurrentThreadId")

	iidIUnknown            = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidClassFactory        = windows.GUID{Data1: 0x00000001, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidExecuteCommand      = windows.GUID{Data1: 0x7F9185B0, Data2: 0xCB92, Data3: 0x43C5, Data4: [8]byte{0x80, 0xA9, 0x92, 0x27, 0x7A, 0x4F, 0x7B, 0x54}}
	iidObjectWithSelection = windows.GUID{Data1: 0x1C9CD5BB, Data2: 0x98E9, Data3: 0x4491, Data4: [8]byte{0xA6, 0x0F, 0x31, 0xAA, 0xCC, 0x72, 0xB8, 0x3C}}
	clsidCopyPair          = windows.GUID{Data1: 0x6C1D6A92, Data2: 0x8E2B, Data3: 0x4B5C, Data4: [8]byte{0x9F, 0x17, 0x1D, 0x6A, 0x31, 0x70, 0xC8, 0xE4}}

	comMainThread uint32
	liveObjects   sync.Map
)

type classFactoryVtbl struct {
	queryInterface, addRef, release, createInstance, lockServer uintptr
}
type classFactory struct {
	vtbl *classFactoryVtbl
	refs int32
}

type executeCommandVtbl struct {
	queryInterface, addRef, release                                                            uintptr
	setKeyState, setParameters, setPosition, setShowWindow, setNoShowUI, setDirectory, execute uintptr
}
type objectSelectionVtbl struct {
	queryInterface, addRef, release, setSelection, getSelection uintptr
}
type executeIface struct {
	vtbl  *executeCommandVtbl
	owner *commandObject
}
type selectionIface struct {
	vtbl  *objectSelectionVtbl
	owner *commandObject
}
type commandObject struct {
	execute   executeIface
	selection selectionIface
	refs      int32
	mu        sync.Mutex
	paths     []string
}

var (
	cfVtbl = classFactoryVtbl{
		syscall.NewCallback(cfQueryInterface), syscall.NewCallback(cfAddRef),
		syscall.NewCallback(cfRelease), syscall.NewCallback(cfCreateInstance),
		syscall.NewCallback(cfLockServer),
	}
	cmdVtbl = executeCommandVtbl{
		syscall.NewCallback(cmdQueryInterface), syscall.NewCallback(cmdAddRef), syscall.NewCallback(cmdRelease),
		syscall.NewCallback(cmdNoop1), syscall.NewCallback(cmdNoop1), syscall.NewCallback(cmdNoop1),
		syscall.NewCallback(cmdNoop1), syscall.NewCallback(cmdNoop1), syscall.NewCallback(cmdNoop1),
		syscall.NewCallback(cmdExecute),
	}
	selVtbl = objectSelectionVtbl{
		syscall.NewCallback(selQueryInterface), syscall.NewCallback(selAddRef), syscall.NewCallback(selRelease),
		syscall.NewCallback(selSetSelection), syscall.NewCallback(selGetSelection),
	}
	globalFactory = &classFactory{vtbl: &cfVtbl, refs: 1}
)

func guidEqual(a, b *windows.GUID) bool {
	return a != nil && b != nil && *a == *b
}

func setInterface(ppv uintptr, value uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = value
	return sOK
}

func cfQueryInterface(this, riid, ppv uintptr) uintptr {
	id := (*windows.GUID)(unsafe.Pointer(riid))
	if guidEqual(id, &iidIUnknown) || guidEqual(id, &iidClassFactory) {
		setInterface(ppv, this)
		cfAddRef(this)
		return sOK
	}
	setInterface(ppv, 0)
	return eNoInterface
}
func cfAddRef(this uintptr) uintptr {
	return uintptr(atomic.AddInt32(&(*classFactory)(unsafe.Pointer(this)).refs, 1))
}
func cfRelease(this uintptr) uintptr {
	return uintptr(atomic.AddInt32(&(*classFactory)(unsafe.Pointer(this)).refs, -1))
}
func cfCreateInstance(this, outer, riid, ppv uintptr) uintptr {
	if outer != 0 {
		return classENoAgg
	}
	obj := &commandObject{}
	obj.execute = executeIface{vtbl: &cmdVtbl, owner: obj}
	obj.selection = selectionIface{vtbl: &selVtbl, owner: obj}
	liveObjects.Store(obj, struct{}{})
	hr := objectQueryInterface(obj, (*windows.GUID)(unsafe.Pointer(riid)), ppv)
	if hr != sOK {
		liveObjects.Delete(obj)
	}
	return hr
}
func cfLockServer(this, lock uintptr) uintptr { return sOK }

func objectQueryInterface(obj *commandObject, id *windows.GUID, ppv uintptr) uintptr {
	if guidEqual(id, &iidIUnknown) || guidEqual(id, &iidExecuteCommand) {
		setInterface(ppv, uintptr(unsafe.Pointer(&obj.execute)))
		atomic.AddInt32(&obj.refs, 1)
		return sOK
	}
	if guidEqual(id, &iidObjectWithSelection) {
		setInterface(ppv, uintptr(unsafe.Pointer(&obj.selection)))
		atomic.AddInt32(&obj.refs, 1)
		return sOK
	}
	setInterface(ppv, 0)
	return eNoInterface
}

func cmdOwner(this uintptr) *commandObject { return (*executeIface)(unsafe.Pointer(this)).owner }
func selOwner(this uintptr) *commandObject { return (*selectionIface)(unsafe.Pointer(this)).owner }

func cmdQueryInterface(this, riid, ppv uintptr) uintptr {
	return objectQueryInterface(cmdOwner(this), (*windows.GUID)(unsafe.Pointer(riid)), ppv)
}
func cmdAddRef(this uintptr) uintptr { return uintptr(atomic.AddInt32(&cmdOwner(this).refs, 1)) }
func cmdRelease(this uintptr) uintptr {
	obj := cmdOwner(this)
	n := atomic.AddInt32(&obj.refs, -1)
	if n == 0 {
		liveObjects.Delete(obj)
	}
	return uintptr(n)
}
func cmdNoop1(this, value uintptr) uintptr { return sOK }

func selQueryInterface(this, riid, ppv uintptr) uintptr {
	return objectQueryInterface(selOwner(this), (*windows.GUID)(unsafe.Pointer(riid)), ppv)
}
func selAddRef(this uintptr) uintptr { return uintptr(atomic.AddInt32(&selOwner(this).refs, 1)) }
func selRelease(this uintptr) uintptr {
	obj := selOwner(this)
	n := atomic.AddInt32(&obj.refs, -1)
	if n == 0 {
		liveObjects.Delete(obj)
	}
	return uintptr(n)
}

func selSetSelection(this, itemArray uintptr) uintptr {
	obj := selOwner(this)
	paths := shellItemArrayPaths(itemArray)
	obj.mu.Lock()
	obj.paths = paths
	obj.mu.Unlock()
	return sOK
}
func selGetSelection(this, riid, ppv uintptr) uintptr {
	if ppv != 0 {
		*(*uintptr)(unsafe.Pointer(ppv)) = 0
	}
	return eNotImpl
}

func cmdExecute(this uintptr) uintptr {
	obj := cmdOwner(this)
	obj.mu.Lock()
	paths := append([]string(nil), obj.paths...)
	obj.mu.Unlock()
	if len(paths) == 0 {
		return sOK
	}

	list, err := os.CreateTemp("", "CopyPairFolder_selection_*.txt")
	if err == nil {
		_, _ = list.WriteString(strings.Join(paths, "\n"))
		_ = list.Close()
		if exe, exeErr := os.Executable(); exeErr == nil {
			cmd := exec.Command(exe, "/process-list", list.Name())
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if startErr := cmd.Start(); startErr != nil {
				_ = os.Remove(list.Name())
			}
		}
	}

	// 本次调用交给工作进程后退出 COM 服务器。
	go func(thread uint32) {
		time.Sleep(1200 * time.Millisecond)
		procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0)
	}(comMainThread)
	return sOK
}

func shellItemArrayPaths(array uintptr) []string {
	if array == 0 {
		return nil
	}
	var count uint32
	if hr := comCall(array, 7, uintptr(unsafe.Pointer(&count))); hr != sOK {
		return nil
	}
	paths := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		var item uintptr
		if comCall(array, 8, uintptr(i), uintptr(unsafe.Pointer(&item))) != sOK || item == 0 {
			continue
		}
		var raw uintptr
		if comCall(item, 5, sigdnFileSysPath, uintptr(unsafe.Pointer(&raw))) == sOK && raw != 0 {
			path := windows.UTF16PtrToString((*uint16)(unsafe.Pointer(raw)))
			procCoTaskMemFree.Call(raw)
			if path != "" {
				paths = append(paths, filepath.Clean(path))
			}
		}
		comCall(item, 2)
	}
	return paths
}

func comCall(object uintptr, method int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(object))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(method)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, object)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

type comMSG struct {
	hwnd    uintptr
	message uint32
	_       uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
	private uint32
}

func runCOMServer() {
	r, _, _ := procCoInitializeEx.Call(0, coinitApartment)
	if r != sOK && r != 1 {
		return
	}
	defer procCoUninitialize.Call()

	tid, _, _ := procGetCurrentThreadID.Call()
	comMainThread = uint32(tid)

	var cookie uint32
	hr, _, _ := procCoRegisterClassObject.Call(
		uintptr(unsafe.Pointer(&clsidCopyPair)),
		uintptr(unsafe.Pointer(globalFactory)),
		clsctxLocalServer,
		regclsMultipleUse,
		uintptr(unsafe.Pointer(&cookie)),
	)
	if hr != sOK {
		return
	}
	defer procCoRevokeClassObject.Call(uintptr(cookie))

	var msg comMSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
