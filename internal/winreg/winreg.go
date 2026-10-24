package winreg

import (
	"syscall"
	"unsafe"
)

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	regOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	regCloseKey      = advapi32.NewProc("RegCloseKey")
	regQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	regCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	regSetValueExW   = advapi32.NewProc("RegSetValueExW")
	regDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	regDeleteKeyW    = advapi32.NewProc("RegDeleteKeyW")
)

const (
	HKEY_CURRENT_USER uintptr = 0x80000001
	KEY_READ          uint32  = 0x20019
	KEY_WRITE         uint32  = 0x20006
	KEY_ALL_ACCESS    uint32  = 0xF003F
	REG_SZ            uint32  = 1
	ERROR_SUCCESS     uintptr = 0
)

// KeyExists checks if an HKCU subkey exists without spawning any process or window
func KeyExists(subKey string) bool {
	uKey, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return false
	}
	var hKey uintptr
	r, _, _ := regOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(uKey)), 0, uintptr(KEY_READ), uintptr(unsafe.Pointer(&hKey)))
	if r == ERROR_SUCCESS {
		_, _, _ = regCloseKey.Call(hKey)
		return true
	}
	return false
}

// ValueExists checks if a specific named value exists in an HKCU subkey
func ValueExists(subKey, valueName string) bool {
	uKey, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return false
	}
	var hKey uintptr
	r, _, _ := regOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(uKey)), 0, uintptr(KEY_READ), uintptr(unsafe.Pointer(&hKey)))
	if r != ERROR_SUCCESS {
		return false
	}
	defer regCloseKey.Call(hKey)

	uVal, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return false
	}
	r, _, _ = regQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(uVal)), 0, 0, 0, 0)
	return r == ERROR_SUCCESS
}

// SetStringValue sets a string (REG_SZ) value in HKCU, creating the subkey if it doesn't exist
func SetStringValue(subKey, valueName, data string) error {
	uKey, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return err
	}
	var hKey uintptr
	r, _, callErr := regCreateKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(uKey)), 0, 0, 0, uintptr(KEY_ALL_ACCESS), 0, uintptr(unsafe.Pointer(&hKey)), 0)
	if r != ERROR_SUCCESS {
		return callErr
	}
	defer regCloseKey.Call(hKey)

	var uVal *uint16
	if valueName != "" {
		uVal, err = syscall.UTF16PtrFromString(valueName)
		if err != nil {
			return err
		}
	}

	uData, err := syscall.UTF16FromString(data)
	if err != nil {
		return err
	}

	r, _, callErr = regSetValueExW.Call(
		hKey,
		uintptr(unsafe.Pointer(uVal)),
		0,
		uintptr(REG_SZ),
		uintptr(unsafe.Pointer(&uData[0])),
		uintptr(len(uData)*2),
	)
	if r != ERROR_SUCCESS {
		return callErr
	}
	return nil
}

// DeleteValue removes a named value from an HKCU subkey
func DeleteValue(subKey, valueName string) error {
	uKey, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return err
	}
	var hKey uintptr
	r, _, callErr := regOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(uKey)), 0, uintptr(KEY_WRITE), uintptr(unsafe.Pointer(&hKey)))
	if r != ERROR_SUCCESS {
		return callErr
	}
	defer regCloseKey.Call(hKey)

	uVal, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return err
	}
	r, _, callErr = regDeleteValueW.Call(hKey, uintptr(unsafe.Pointer(uVal)))
	if r != ERROR_SUCCESS {
		return callErr
	}
	return nil
}

// DeleteKey removes an entire subkey from HKCU
func DeleteKey(subKey string) error {
	uKey, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return err
	}
	r, _, callErr := regDeleteKeyW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(uKey)))
	if r != ERROR_SUCCESS {
		return callErr
	}
	return nil
}
