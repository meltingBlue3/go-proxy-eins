//go:build windows

package sysproxy

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

// ProxyConfig 代理配置信息
type ProxyConfig struct {
	Enabled       bool
	Server        string
	Override      string
	AutoConfigURL string
}

// GetCurrentProxy 获取当前系统代理设置
func GetCurrentProxy() (*ProxyConfig, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	config := &ProxyConfig{}

	// 读取 ProxyEnable
	if val, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		config.Enabled = (val != 0)
	}

	// 读取 ProxyServer
	if val, _, err := key.GetStringValue("ProxyServer"); err == nil {
		config.Server = val
	}

	// 读取 ProxyOverride
	if val, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		config.Override = val
	}

	// 读取 AutoConfigURL
	if val, _, err := key.GetStringValue("AutoConfigURL"); err == nil {
		config.AutoConfigURL = val
	}

	return config, nil
}

// SetHTTPProxy 设置系统 HTTP 代理
func SetHTTPProxy(addr string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// 启用代理
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("failed to set ProxyEnable: %w", err)
	}

	// 设置代理服务器地址
	if err := key.SetStringValue("ProxyServer", addr); err != nil {
		return fmt.Errorf("failed to set ProxyServer: %w", err)
	}

	// 设置代理排除列表（本地地址不走代理）
	override := "localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;<local>"
	if err := key.SetStringValue("ProxyOverride", override); err != nil {
		return fmt.Errorf("failed to set ProxyOverride: %w", err)
	}

	// 通知系统代理设置已更改
	if err := notifyProxyChange(); err != nil {
		return fmt.Errorf("failed to notify proxy change: %w", err)
	}

	return nil
}

// RestoreProxy 恢复代理设置
func RestoreProxy(config *ProxyConfig) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// 恢复 ProxyEnable
	var enableVal uint32 = 0
	if config.Enabled {
		enableVal = 1
	}
	if err := key.SetDWordValue("ProxyEnable", enableVal); err != nil {
		return fmt.Errorf("failed to restore ProxyEnable: %w", err)
	}

	// 恢复 ProxyServer
	if config.Server != "" {
		if err := key.SetStringValue("ProxyServer", config.Server); err != nil {
			return fmt.Errorf("failed to restore ProxyServer: %w", err)
		}
	} else {
		// 如果原来没有设置，删除该值
		key.DeleteValue("ProxyServer")
	}

	// 恢复 ProxyOverride
	if config.Override != "" {
		if err := key.SetStringValue("ProxyOverride", config.Override); err != nil {
			return fmt.Errorf("failed to restore ProxyOverride: %w", err)
		}
	} else {
		key.DeleteValue("ProxyOverride")
	}

	// 恢复 AutoConfigURL
	if config.AutoConfigURL != "" {
		if err := key.SetStringValue("AutoConfigURL", config.AutoConfigURL); err != nil {
			return fmt.Errorf("failed to restore AutoConfigURL: %w", err)
		}
	} else {
		key.DeleteValue("AutoConfigURL")
	}

	// 通知系统代理设置已更改
	if err := notifyProxyChange(); err != nil {
		return fmt.Errorf("failed to notify proxy change: %w", err)
	}

	return nil
}

// DisableProxy 禁用系统代理
func DisableProxy() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// 禁用代理
	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		return fmt.Errorf("failed to disable proxy: %w", err)
	}

	// 通知系统代理设置已更改
	if err := notifyProxyChange(); err != nil {
		return fmt.Errorf("failed to notify proxy change: %w", err)
	}

	return nil
}

// notifyProxyChange 通知系统代理设置已更改
func notifyProxyChange() error {
	// 使用 Windows API 通知系统刷新 Internet 设置
	// 这会让浏览器和其他应用程序立即感知代理更改
	return notifyWinInetProxyChange()
}
