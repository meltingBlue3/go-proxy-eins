//go:build windows

package sysproxy

import (
	"syscall"
)

var (
	wininet                    = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOptionW     = wininet.NewProc("InternetSetOptionW")
)

const (
	INTERNET_OPTION_SETTINGS_CHANGED = 39
	INTERNET_OPTION_REFRESH          = 37
)

// notifyWinInetProxyChange 通知 Windows Internet 设置已更改
func notifyWinInetProxyChange() error {
	// 通知设置已更改
	ret, _, _ := procInternetSetOptionW.Call(
		0,
		INTERNET_OPTION_SETTINGS_CHANGED,
		0,
		0,
	)
	if ret == 0 {
		return syscall.GetLastError()
	}

	// 刷新设置
	ret, _, _ = procInternetSetOptionW.Call(
		0,
		INTERNET_OPTION_REFRESH,
		0,
		0,
	)
	if ret == 0 {
		return syscall.GetLastError()
	}

	return nil
}

