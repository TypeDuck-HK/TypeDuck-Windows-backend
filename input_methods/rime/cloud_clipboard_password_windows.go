//go:build windows

package rime

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modCrypt32           = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData = modCrypt32.NewProc("CryptProtectData")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func cloudClipboardPasswordPath() string {
	root := moqiAppDataDir()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "cloud_clipboard.pw")
}

func readCloudClipboardPassword() string {
	path := cloudClipboardPasswordPath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	plain, err := unprotectData(data)
	if err != nil {
		return ""
	}
	return string(plain)
}

func saveCloudClipboardPassword(password string) error {
	path := cloudClipboardPasswordPath()
	if path == "" {
		return fmt.Errorf("无法确定配置目录")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if password == "" {
		return os.Remove(path)
	}
	protected, err := protectData([]byte(password))
	if err != nil {
		return err
	}
	return os.WriteFile(path, protected, 0o600)
}

func protectData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer localFree(out.pbData)
	return cloneBytes(out.pbData, out.cbData), nil
}

func unprotectData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	proc := modCrypt32.NewProc("CryptUnprotectData")
	r, _, err := proc.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer localFree(out.pbData)
	return cloneBytes(out.pbData, out.cbData), nil
}

func localFree(ptr *byte) {
	modKernel32 := syscall.NewLazyDLL("kernel32.dll")
	procLocalFree := modKernel32.NewProc("LocalFree")
	procLocalFree.Call(uintptr(unsafe.Pointer(ptr)))
}

func cloneBytes(ptr *byte, size uint32) []byte {
	if ptr == nil || size == 0 {
		return nil
	}
	out := make([]byte, size)
	copy(out, unsafe.Slice(ptr, size))
	return out
}
