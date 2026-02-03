//go:build linux

package sysproxy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ProxyConfig 代理配置信息
type ProxyConfig struct {
	Enabled       bool
	Server        string
	Override      string
	AutoConfigURL string
}

// detectDesktopEnvironment 检测当前桌面环境
func detectDesktopEnvironment() string {
	// 检查常见的桌面环境环境变量
	if os.Getenv("GNOME_DESKTOP_SESSION_ID") != "" || os.Getenv("GNOME_SHELL_SESSION_MODE") != "" {
		return "gnome"
	}
	if os.Getenv("KDE_FULL_SESSION") != "" || os.Getenv("KDE_SESSION_VERSION") != "" {
		return "kde"
	}
	
	// 检查 XDG_CURRENT_DESKTOP
	desktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	if strings.Contains(desktop, "gnome") {
		return "gnome"
	}
	if strings.Contains(desktop, "kde") || strings.Contains(desktop, "plasma") {
		return "kde"
	}
	
	// 检查 DESKTOP_SESSION
	session := strings.ToLower(os.Getenv("DESKTOP_SESSION"))
	if strings.Contains(session, "gnome") {
		return "gnome"
	}
	if strings.Contains(session, "kde") || strings.Contains(session, "plasma") {
		return "kde"
	}
	
	return "unknown"
}

// GetCurrentProxy 获取当前系统代理设置
func GetCurrentProxy() (*ProxyConfig, error) {
	de := detectDesktopEnvironment()
	
	switch de {
	case "gnome":
		return getGNOMEProxy()
	case "kde":
		return getKDEProxy()
	default:
		// 对于未知环境，返回空配置
		return &ProxyConfig{
			Enabled: false,
			Server:  "",
		}, nil
	}
}

// getGNOMEProxy 获取 GNOME 代理设置
func getGNOMEProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}
	
	// 检查是否启用代理
	cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode")
	output, err := cmd.Output()
	if err != nil {
		// gsettings 不可用
		return config, nil
	}
	
	mode := strings.Trim(string(output), "'\n ")
	config.Enabled = (mode == "manual")
	
	if config.Enabled {
		// 获取 HTTP 代理服务器
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "host")
		if output, err := cmd.Output(); err == nil {
			host := strings.Trim(string(output), "'\n ")
			
			// 获取端口
			cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy.http", "port")
			if portOutput, err := cmd.Output(); err == nil {
				port := strings.Trim(string(portOutput), "'\n ")
				if host != "" && port != "" {
					config.Server = fmt.Sprintf("%s:%s", host, port)
				}
			}
		}
		
		// 获取忽略列表
		cmd = exec.Command("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts")
		if output, err := cmd.Output(); err == nil {
			config.Override = strings.Trim(string(output), "'\n ")
		}
	}
	
	return config, nil
}

// getKDEProxy 获取 KDE 代理设置
func getKDEProxy() (*ProxyConfig, error) {
	config := &ProxyConfig{}
	
	// KDE 代理配置存储在 ~/.config/kioslaverc
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return config, nil
	}
	
	configPath := homeDir + "/.config/kioslaverc"
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 配置文件不存在
		return config, nil
	}
	
	// 简单解析 INI 格式
	lines := strings.Split(string(data), "\n")
	inProxySection := false
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if line == "[Proxy Settings]" {
			inProxySection = true
			continue
		}
		
		if strings.HasPrefix(line, "[") {
			inProxySection = false
			continue
		}
		
		if inProxySection {
			if strings.HasPrefix(line, "ProxyType=") {
				value := strings.TrimPrefix(line, "ProxyType=")
				// 1 = 手动代理
				config.Enabled = (value == "1")
			} else if strings.HasPrefix(line, "httpProxy=") {
				config.Server = strings.TrimPrefix(line, "httpProxy=")
			} else if strings.HasPrefix(line, "NoProxyFor=") {
				config.Override = strings.TrimPrefix(line, "NoProxyFor=")
			}
		}
	}
	
	return config, nil
}

// SetHTTPProxy 设置系统 HTTP 代理
func SetHTTPProxy(addr string) error {
	de := detectDesktopEnvironment()
	
	switch de {
	case "gnome":
		return setGNOMEProxy(addr)
	case "kde":
		return setKDEProxy(addr)
	default:
		// 对于未知环境，返回友好的错误消息
		return fmt.Errorf("automatic proxy configuration is not supported on this desktop environment.\n" +
			"Please manually configure your system/browser to use HTTP proxy: %s", addr)
	}
}

// setGNOMEProxy 设置 GNOME 代理
func setGNOMEProxy(addr string) error {
	// 检查 gsettings 是否可用
	if _, err := exec.LookPath("gsettings"); err != nil {
		return fmt.Errorf("gsettings not found. Please manually configure your proxy to: %s", addr)
	}
	
	// 解析地址
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid proxy address format: %s", addr)
	}
	host := parts[0]
	port := parts[1]
	
	// 设置代理模式为手动
	cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set proxy mode: %w", err)
	}
	
	// 设置 HTTP 代理主机
	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set proxy host: %w", err)
	}
	
	// 设置 HTTP 代理端口
	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set proxy port: %w", err)
	}
	
	// 同时设置 HTTPS 代理
	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", host)
	cmd.Run()
	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", port)
	cmd.Run()
	
	// 设置忽略列表（本地地址不走代理）
	ignoreHosts := "['localhost', '127.0.0.0/8', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']"
	cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy", "ignore-hosts", ignoreHosts)
	if err := cmd.Run(); err != nil {
		// 忽略此错误，不是致命的
	}
	
	return nil
}

// setKDEProxy 设置 KDE 代理
func setKDEProxy(addr string) error {
	// 检查 kwriteconfig5 是否可用
	kwriteconfig := "kwriteconfig5"
	if _, err := exec.LookPath(kwriteconfig); err != nil {
		// 尝试 kwriteconfig (旧版本)
		if _, err := exec.LookPath("kwriteconfig"); err != nil {
			return fmt.Errorf("kwriteconfig not found. Please manually configure your proxy to: %s", addr)
		}
		kwriteconfig = "kwriteconfig"
	}
	
	// 设置代理类型为手动 (1)
	cmd := exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set proxy type: %w", err)
	}
	
	// 设置 HTTP 代理
	cmd = exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", "http://"+addr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set HTTP proxy: %w", err)
	}
	
	// 设置 HTTPS 代理
	cmd = exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpsProxy", "http://"+addr)
	cmd.Run()
	
	// 设置忽略列表
	cmd = exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "NoProxyFor", 
		"localhost,127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")
	cmd.Run()
	
	// 通知 KDE 重新加载配置
	cmd = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:''")
	cmd.Run()
	
	return nil
}

// RestoreProxy 恢复代理设置
func RestoreProxy(config *ProxyConfig) error {
	if config == nil {
		return DisableProxy()
	}
	
	de := detectDesktopEnvironment()
	
	switch de {
	case "gnome":
		return restoreGNOMEProxy(config)
	case "kde":
		return restoreKDEProxy(config)
	default:
		return nil
	}
}

// restoreGNOMEProxy 恢复 GNOME 代理设置
func restoreGNOMEProxy(config *ProxyConfig) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return nil
	}
	
	if !config.Enabled {
		// 禁用代理
		cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
		return cmd.Run()
	}
	
	// 恢复手动代理
	if config.Server != "" {
		parts := strings.Split(config.Server, ":")
		if len(parts) == 2 {
			host := parts[0]
			port := parts[1]
			
			cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
			cmd.Run()
			
			cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host)
			cmd.Run()
			
			cmd = exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port)
			cmd.Run()
		}
	}
	
	return nil
}

// restoreKDEProxy 恢复 KDE 代理设置
func restoreKDEProxy(config *ProxyConfig) error {
	kwriteconfig := "kwriteconfig5"
	if _, err := exec.LookPath(kwriteconfig); err != nil {
		if _, err := exec.LookPath("kwriteconfig"); err != nil {
			return nil
		}
		kwriteconfig = "kwriteconfig"
	}
	
	if !config.Enabled {
		// 禁用代理 (ProxyType=0)
		cmd := exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0")
		return cmd.Run()
	}
	
	// 恢复手动代理
	if config.Server != "" {
		cmd := exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "1")
		cmd.Run()
		
		cmd = exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "httpProxy", config.Server)
		cmd.Run()
	}
	
	return nil
}

// DisableProxy 禁用系统代理
func DisableProxy() error {
	de := detectDesktopEnvironment()
	
	switch de {
	case "gnome":
		if _, err := exec.LookPath("gsettings"); err != nil {
			return nil
		}
		cmd := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none")
		return cmd.Run()
		
	case "kde":
		kwriteconfig := "kwriteconfig5"
		if _, err := exec.LookPath(kwriteconfig); err != nil {
			if _, err := exec.LookPath("kwriteconfig"); err != nil {
				return nil
			}
			kwriteconfig = "kwriteconfig"
		}
		cmd := exec.Command(kwriteconfig, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", "ProxyType", "0")
		return cmd.Run()
		
	default:
		return nil
	}
}
