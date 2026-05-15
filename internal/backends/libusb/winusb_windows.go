//go:build windows

// Package libusb provides direct USB I/O for printers whose driver has
// been replaced with WinUSB (e.g. by the legacy POS Printer Driver
// Installer or Zadig). Communication goes through the Win32 WinUSB API,
// not via the Windows print spooler — useful when usbprint.sys is not
// available or has been intentionally swapped out for WebUSB compatibility.
//
// This implementation is pure Go: it calls setupapi.dll, winusb.dll and
// kernel32.dll via the `golang.org/x/sys/windows` lazy-DLL infrastructure.
// No libusb, no CGO, no C toolchain required at build time.
package libusb

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Device is one WinUSB-bound USB device that looks like a thermal printer.
type Device struct {
	InstanceID string // e.g. USB\VID_04B8&PID_0202\6&1234
	VID        string // 04B8
	PID        string // 0202
	Service    string // typically "WinUsb"
	Path       string // \\?\usb#vid_04b8&pid_0202#... — used by CreateFile
}

// List enumerates USB devices currently bound to the WinUsb service.
// Each returned Device is ready to be passed to Print().
func List() ([]Device, error) {
	devs, err := enumWinusbDevices()
	if err != nil {
		return nil, err
	}
	return devs, nil
}

// Print writes raw bytes to the printer's first bulk-OUT endpoint.
//
// Flow:
//   1. CreateFile on the device path (FILE_FLAG_OVERLAPPED required by WinUSB).
//   2. WinUsb_Initialize → InterfaceHandle.
//   3. WinUsb_QueryInterfaceSettings to learn how many endpoints the iface has.
//   4. WinUsb_QueryPipe for each endpoint; pick the first OUT bulk pipe.
//   5. WinUsb_WritePipe with the data.
//   6. Close handles in reverse order.
func Print(dev Device, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, errors.New("payload vide")
	}
	if dev.Path == "" {
		return 0, errors.New("device path manquant")
	}

	pathPtr, err := windows.UTF16PtrFromString(dev.Path)
	if err != nil {
		return 0, err
	}

	const (
		genericRead       = 0x80000000
		genericWrite      = 0x40000000
		fileShareRead     = 0x00000001
		fileShareWrite    = 0x00000002
		openExisting      = 3
		fileAttrNormal    = 0x00000080
		fileFlagOverlapped = 0x40000000
	)
	handle, err := windows.CreateFile(
		pathPtr,
		genericRead|genericWrite,
		fileShareRead|fileShareWrite,
		nil,
		openExisting,
		fileAttrNormal|fileFlagOverlapped,
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateFile %q : %w", dev.Path, err)
	}
	defer windows.CloseHandle(handle)

	var winusbHandle uintptr
	r, _, errc := procWinUsbInitialize.Call(uintptr(handle), uintptr(unsafe.Pointer(&winusbHandle)))
	if r == 0 {
		return 0, fmt.Errorf("WinUsb_Initialize : %w", errc)
	}
	defer procWinUsbFree.Call(winusbHandle)

	var ifaceDesc usbInterfaceDescriptor
	r, _, errc = procWinUsbQueryInterfaceSettings.Call(
		winusbHandle, 0, uintptr(unsafe.Pointer(&ifaceDesc)),
	)
	if r == 0 {
		return 0, fmt.Errorf("WinUsb_QueryInterfaceSettings : %w", errc)
	}

	// Find first BULK OUT endpoint.
	var outPipeID byte = 0
	for i := byte(0); i < ifaceDesc.NumEndpoints; i++ {
		var pipe winusbPipeInformation
		r, _, _ = procWinUsbQueryPipe.Call(
			winusbHandle, 0, uintptr(i), uintptr(unsafe.Pointer(&pipe)),
		)
		if r == 0 {
			continue
		}
		// PipeType 2 = USBD_PIPE_TYPE_BULK; high bit of pipeID clear = OUT.
		if pipe.PipeType == 2 && (pipe.PipeId&0x80) == 0 {
			outPipeID = pipe.PipeId
			break
		}
	}
	if outPipeID == 0 {
		return 0, errors.New("aucun endpoint BULK OUT trouvé sur l'interface WinUSB")
	}

	var transferred uint32
	r, _, errc = procWinUsbWritePipe.Call(
		winusbHandle,
		uintptr(outPipeID),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&transferred)),
		0,
	)
	if r == 0 {
		return int(transferred), fmt.Errorf("WinUsb_WritePipe : %w", errc)
	}
	return int(transferred), nil
}

// --- Internals: SetupAPI enumeration --------------------------------------

var (
	setupapi = windows.NewLazySystemDLL("setupapi.dll")
	winusbDLL = windows.NewLazySystemDLL("winusb.dll")

	procSetupDiGetClassDevsW                = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo               = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceRegistryPropertyW   = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiGetDeviceInstanceIdW         = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiEnumDeviceInterfaces         = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW    = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList        = setupapi.NewProc("SetupDiDestroyDeviceInfoList")

	procWinUsbInitialize             = winusbDLL.NewProc("WinUsb_Initialize")
	procWinUsbFree                   = winusbDLL.NewProc("WinUsb_Free")
	procWinUsbQueryInterfaceSettings = winusbDLL.NewProc("WinUsb_QueryInterfaceSettings")
	procWinUsbQueryPipe              = winusbDLL.NewProc("WinUsb_QueryPipe")
	procWinUsbWritePipe              = winusbDLL.NewProc("WinUsb_WritePipe")
)

const (
	digcfPresent         = 0x00000002
	digcfAllClasses      = 0x00000004
	digcfDeviceInterface = 0x00000010

	spdrpService    = 0x00000004
	spdrpHardwareID = 0x00000001
)

type spDevInfoData struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

type spDeviceInterfaceData struct {
	CbSize             uint32
	InterfaceClassGuid windows.GUID
	Flags              uint32
	Reserved           uintptr
}

type usbInterfaceDescriptor struct {
	BLength            byte
	BDescriptorType    byte
	BInterfaceNumber   byte
	BAlternateSetting  byte
	NumEndpoints       byte
	BInterfaceClass    byte
	BInterfaceSubClass byte
	BInterfaceProtocol byte
	IInterface         byte
}

type winusbPipeInformation struct {
	PipeType       uint32
	PipeId         byte
	_              [3]byte
	MaximumPacketSize uint16
	Interval       byte
	_              [1]byte
}

// All-classes enumeration returns every present device. We filter to USB by
// hardware-ID prefix and service == "WinUsb".
func enumWinusbDevices() ([]Device, error) {
	devInfo, _, _ := procSetupDiGetClassDevsW.Call(
		0, // guid = NULL → all classes
		0, // enumerator = NULL
		0, // hwndParent = NULL
		uintptr(digcfPresent|digcfAllClasses),
	)
	if devInfo == 0 || int32(devInfo) == -1 {
		return nil, errors.New("SetupDiGetClassDevs failed")
	}
	defer procSetupDiDestroyDeviceInfoList.Call(devInfo)

	var out []Device
	var idx uint32
	for {
		var did spDevInfoData
		did.CbSize = uint32(unsafe.Sizeof(did))
		r, _, _ := procSetupDiEnumDeviceInfo.Call(
			devInfo, uintptr(idx), uintptr(unsafe.Pointer(&did)),
		)
		if r == 0 {
			break
		}
		idx++

		svc, _ := readStringProperty(devInfo, &did, spdrpService)
		if !strings.EqualFold(svc, "WinUsb") {
			continue
		}
		hwid, _ := readMultiStringProperty(devInfo, &did, spdrpHardwareID)
		if !isUSBHardwareID(hwid) {
			continue
		}
		instanceID, _ := readDeviceInstanceID(devInfo, &did)
		vid, pid := parseVIDPID(instanceID + " " + strings.Join(hwid, " "))

		// Try to find this device's device-interface path. WinUSB
		// stores its interface GUID(s) under
		//   HKLM\SYSTEM\CurrentControlSet\Enum\USB\<vidpid>\<inst>\Device Parameters
		path := findInterfacePath(devInfo, &did, instanceID)
		if path == "" {
			continue
		}

		out = append(out, Device{
			InstanceID: instanceID,
			VID:        vid,
			PID:        pid,
			Service:    svc,
			Path:       path,
		})
	}
	return out, nil
}

func readStringProperty(devInfo uintptr, did *spDevInfoData, prop uint32) (string, error) {
	buf := make([]uint16, 256)
	var size uint32 = uint32(len(buf) * 2)
	r, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		devInfo,
		uintptr(unsafe.Pointer(did)),
		uintptr(prop),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return "", syscall.GetLastError()
	}
	return windows.UTF16ToString(buf), nil
}

func readMultiStringProperty(devInfo uintptr, did *spDevInfoData, prop uint32) ([]string, error) {
	buf := make([]uint16, 1024)
	var size uint32 = uint32(len(buf) * 2)
	r, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		devInfo,
		uintptr(unsafe.Pointer(did)),
		uintptr(prop),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return nil, syscall.GetLastError()
	}
	// Multi-sz: sequence of UTF-16 strings terminated by an empty one.
	var parts []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			parts = append(parts, windows.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	return parts, nil
}

func readDeviceInstanceID(devInfo uintptr, did *spDevInfoData) (string, error) {
	buf := make([]uint16, 512)
	var size uint32 = uint32(len(buf))
	r, _, _ := procSetupDiGetDeviceInstanceIdW.Call(
		devInfo,
		uintptr(unsafe.Pointer(did)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return "", syscall.GetLastError()
	}
	return windows.UTF16ToString(buf), nil
}

func isUSBHardwareID(ids []string) bool {
	for _, h := range ids {
		if strings.HasPrefix(strings.ToUpper(h), "USB\\VID_") {
			return true
		}
	}
	return false
}

var reVIDPID = regexp.MustCompile(`(?i)VID_([0-9A-F]{4}).{0,3}PID_([0-9A-F]{4})`)

func parseVIDPID(s string) (string, string) {
	m := reVIDPID.FindStringSubmatch(s)
	if len(m) == 3 {
		return strings.ToUpper(m[1]), strings.ToUpper(m[2])
	}
	return "", ""
}

// findInterfacePath enumerates device interfaces using the GUID(s) stored
// at HKLM\SYSTEM\CurrentControlSet\Enum\<instanceID>\Device Parameters,
// and returns the first valid path it finds.
func findInterfacePath(devInfo uintptr, did *spDevInfoData, instanceID string) string {
	regPath := `SYSTEM\CurrentControlSet\Enum\` + instanceID + `\Device Parameters`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, regPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	guids, _, err := key.GetStringsValue("DeviceInterfaceGUIDs")
	if err != nil || len(guids) == 0 {
		// Some libwdi-generated drivers store the value as a single REG_SZ.
		s, _, e2 := key.GetStringValue("DeviceInterfaceGUIDs")
		if e2 != nil || s == "" {
			return ""
		}
		guids = []string{s}
	}
	for _, gs := range guids {
		g, err := windows.GUIDFromString(gs)
		if err != nil {
			continue
		}
		if path := lookupInterfacePath(g, instanceID); path != "" {
			return path
		}
	}
	return ""
}

func lookupInterfacePath(guid windows.GUID, instanceID string) string {
	devInfo, _, _ := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&guid)),
		0,
		0,
		uintptr(digcfPresent|digcfDeviceInterface),
	)
	if devInfo == 0 || int32(devInfo) == -1 {
		return ""
	}
	defer procSetupDiDestroyDeviceInfoList.Call(devInfo)

	var idx uint32
	for {
		var iface spDeviceInterfaceData
		iface.CbSize = uint32(unsafe.Sizeof(iface))
		r, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			devInfo, 0, uintptr(unsafe.Pointer(&guid)),
			uintptr(idx), uintptr(unsafe.Pointer(&iface)),
		)
		if r == 0 {
			return ""
		}
		idx++

		// First call to size buffer
		var required uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(
			devInfo,
			uintptr(unsafe.Pointer(&iface)),
			0, 0,
			uintptr(unsafe.Pointer(&required)),
			0,
		)
		if required < 8 {
			continue
		}

		buf := make([]byte, required)
		// First 4 bytes = CbSize. On 64-bit Windows it's 8.
		if unsafe.Sizeof(uintptr(0)) == 8 {
			*(*uint32)(unsafe.Pointer(&buf[0])) = 8
		} else {
			*(*uint32)(unsafe.Pointer(&buf[0])) = 6
		}

		r, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(
			devInfo,
			uintptr(unsafe.Pointer(&iface)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(required),
			0, 0,
		)
		if r == 0 {
			continue
		}
		// DevicePath starts at offset 4 (32-bit) or 4-on-32, 4-on-64.
		// Actually the docs say offset 4 — the struct is { DWORD CbSize; WCHAR DevicePath[]; }
		// so we always read from offset 4.
		path := utf16BytesToString(buf[4:])
		if path == "" {
			continue
		}
		// We have an interface path now. Sanity check: it should contain the
		// instance's VID/PID. Helps when the same GUID matches multiple devs.
		idLower := strings.ToLower(strings.Replace(instanceID, "\\", "#", -1))
		if !strings.Contains(strings.ToLower(path), strings.ToLower("vid_"+vidFromInstance(instanceID))) {
			_ = idLower
			continue
		}
		return path
	}
}

func vidFromInstance(s string) string {
	v, _ := parseVIDPID(s)
	return v
}

func utf16BytesToString(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	// b is a UTF-16LE byte sequence (no length field, null-terminated).
	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
	return windows.UTF16ToString(u16)
}
