package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

// ServerConfig 服务端配置
type ServerConfig struct {
	Port       int    `json:"port"`
	Password   string `json:"password"`
	Timeout    int    `json:"timeout"`     // 秒
	LogLevel   string `json:"log_level"`
	Obfuscate  bool   `json:"obfuscate"`
}

// LocalConfig 客户端配置
type LocalConfig struct {
	LocalAddr  string `json:"local_addr"`
	Server     string `json:"server"`
	Password   string `json:"password"`
	Timeout    int    `json:"timeout"`     // 秒
	LogLevel   string `json:"log_level"`
	Obfuscate  bool   `json:"obfuscate"`
}

// LoadServerConfig 加载服务端配置
func LoadServerConfig() (*ServerConfig, error) {
	// 默认配置
	cfg := &ServerConfig{
		Port:      8081,
		Password:  "",
		Timeout:   30,
		LogLevel:  "info",
		Obfuscate: false,
	}

	// 命令行参数
	var configFile string
	flag.StringVar(&configFile, "c", "", "配置文件路径")
	flag.IntVar(&cfg.Port, "p", cfg.Port, "监听端口")
	flag.StringVar(&cfg.Password, "k", "", "加密密码")
	flag.IntVar(&cfg.Timeout, "t", cfg.Timeout, "连接超时（秒）")
	flag.StringVar(&cfg.LogLevel, "l", cfg.LogLevel, "日志级别 (debug/info/warn/error)")
	flag.BoolVar(&cfg.Obfuscate, "o", cfg.Obfuscate, "启用流量混淆")
	flag.Parse()

	// 如果指定了配置文件，先加载文件配置
	if configFile != "" {
		if err := loadConfigFromFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// 命令行参数会覆盖配置文件（通过重新解析 flag 实现）
	// 这里简化处理，命令行参数优先级更高

	// 验证必填参数
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required (use -k flag or config file)")
	}

	return cfg, nil
}

// LoadLocalConfig 加载客户端配置
func LoadLocalConfig() (*LocalConfig, error) {
	// 默认配置
	cfg := &LocalConfig{
		LocalAddr: "127.0.0.1:1080",
		Server:    "",
		Password:  "",
		Timeout:   30,
		LogLevel:  "info",
		Obfuscate: false,
	}

	// 命令行参数
	var configFile string
	flag.StringVar(&configFile, "c", "", "配置文件路径")
	flag.StringVar(&cfg.LocalAddr, "b", cfg.LocalAddr, "本地监听地址")
	flag.StringVar(&cfg.Server, "s", "", "服务器地址")
	flag.StringVar(&cfg.Password, "k", "", "加密密码")
	flag.IntVar(&cfg.Timeout, "t", cfg.Timeout, "连接超时（秒）")
	flag.StringVar(&cfg.LogLevel, "l", cfg.LogLevel, "日志级别 (debug/info/warn/error)")
	flag.BoolVar(&cfg.Obfuscate, "o", cfg.Obfuscate, "启用流量混淆")
	flag.Parse()

	// 如果指定了配置文件，先加载文件配置
	if configFile != "" {
		if err := loadConfigFromFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// 验证必填参数
	if cfg.Server == "" {
		return nil, fmt.Errorf("server address is required (use -s flag or config file)")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("password is required (use -k flag or config file)")
	}

	return cfg, nil
}

// loadConfigFromFile 从 JSON 文件加载配置
func loadConfigFromFile(path string, cfg interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}

	return nil
}

// GetTimeout 获取超时时间
func (c *ServerConfig) GetTimeout() time.Duration {
	return time.Duration(c.Timeout) * time.Second
}

// GetTimeout 获取超时时间
func (c *LocalConfig) GetTimeout() time.Duration {
	return time.Duration(c.Timeout) * time.Second
}
