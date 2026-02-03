package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-proxy-eins/internal/cipher"
	"go-proxy-eins/internal/config"
	"go-proxy-eins/internal/httpproxy"
	"go-proxy-eins/internal/logger"
	"go-proxy-eins/internal/protocol"
	"go-proxy-eins/internal/sysproxy"
)

var (
	originalProxyConfig *sysproxy.ProxyConfig
)

func main() {
	// 加载配置
	cfg, err := config.LoadLocalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger.Init(logger.ParseLevel(cfg.LogLevel), os.Stdout)
	logger.Log.Info("Starting local proxy", 
		"socks5", cfg.LocalAddr, 
		"http", cfg.HTTPProxyAddr,
		"server", cfg.Server, 
		"obfuscate", cfg.Obfuscate,
		"auto_proxy", cfg.AutoProxy)

	// 设置系统代理（如果启用）
	if cfg.AutoProxy {
		if err := setupSystemProxy(cfg); err != nil {
			logger.Log.Warn("Failed to setup system proxy", "error", err)
		}
	}

	// 设置信号处理（优雅退出）
	setupSignalHandler(cfg)

	// 启动 SOCKS5 监听器
	go startSOCKS5Listener(cfg)

	// 启动 HTTP 代理监听器（主 goroutine）
	startHTTPProxyListener(cfg)
}

// setupSystemProxy 设置 Windows 系统代理
func setupSystemProxy(cfg *config.LocalConfig) error {
	// 获取当前代理配置
	current, err := sysproxy.GetCurrentProxy()
	if err != nil {
		return fmt.Errorf("failed to get current proxy: %w", err)
	}
	originalProxyConfig = current

	logger.Log.Info("Current proxy settings backed up", 
		"enabled", current.Enabled, 
		"server", current.Server)

	// 设置新的 HTTP 代理
	if err := sysproxy.SetHTTPProxy(cfg.HTTPProxyAddr); err != nil {
		return fmt.Errorf("failed to set HTTP proxy: %w", err)
	}

	logger.Log.Info("System proxy configured", "proxy", cfg.HTTPProxyAddr)
	return nil
}

// setupSignalHandler 设置信号处理器以优雅退出
func setupSignalHandler(cfg *config.LocalConfig) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Log.Info("Received signal, shutting down...", "signal", sig)

		// 恢复系统代理
		if cfg.AutoProxy && originalProxyConfig != nil {
			if err := sysproxy.RestoreProxy(originalProxyConfig); err != nil {
				logger.Log.Error("Failed to restore proxy", "error", err)
			} else {
				logger.Log.Info("System proxy restored")
			}
		}

		os.Exit(0)
	}()
}

// startSOCKS5Listener 启动 SOCKS5 监听器
func startSOCKS5Listener(cfg *config.LocalConfig) {
	listener, err := net.Listen("tcp", cfg.LocalAddr)
	if err != nil {
		logger.Log.Error("Failed to listen on SOCKS5 address", "error", err, "addr", cfg.LocalAddr)
		return
	}
	defer listener.Close()

	logger.Log.Info("SOCKS5 proxy is running", "address", listener.Addr())

	for {
		client, err := listener.Accept()
		if err != nil {
			logger.Log.Warn("Failed to accept SOCKS5 connection", "error", err)
			continue
		}

		go handleSOCKS5(client, cfg)
	}
}

// startHTTPProxyListener 启动 HTTP 代理监听器
func startHTTPProxyListener(cfg *config.LocalConfig) {
	listener, err := net.Listen("tcp", cfg.HTTPProxyAddr)
	if err != nil {
		logger.Log.Error("Failed to listen on HTTP proxy address", "error", err, "addr", cfg.HTTPProxyAddr)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Log.Info("HTTP proxy is running", "address", listener.Addr())

	for {
		client, err := listener.Accept()
		if err != nil {
			logger.Log.Warn("Failed to accept HTTP connection", "error", err)
			continue
		}

		go handleHTTPProxy(client, cfg)
	}
}

// handleHTTPProxy 处理 HTTP 代理连接
func handleHTTPProxy(client net.Conn, cfg *config.LocalConfig) {
	defer client.Close()

	// 设置超时
	if cfg.Timeout > 0 {
		client.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	reader := bufio.NewReader(client)

	// 读取第一行以确定请求类型
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	// 检查是否是 CONNECT 请求
	if len(requestLine) >= 7 && requestLine[:7] == "CONNECT" {
		httpproxy.HandleHTTPConnect(client, reader, requestLine, cfg)
	} else {
		// 其他 HTTP 方法暂不支持（可以扩展）
		httpproxy.HandleHTTPConnect(client, reader, requestLine, cfg)
	}
}

// handleSOCKS5 处理 SOCKS5 连接
func handleSOCKS5(client net.Conn, cfg *config.LocalConfig) {
	defer client.Close()

	// 设置超时
	if cfg.Timeout > 0 {
		client.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	logger.Log.Debug("New SOCKS5 connection", "remote", client.RemoteAddr())

	reader := bufio.NewReader(client)

	// 1. SOCKS5 认证
	ver, err := reader.ReadByte()
	if err != nil {
		return
	}
	nmethods, err := reader.ReadByte()
	if err != nil {
		return
	}
	reader.Discard(int(nmethods)) // 跳过 methods

	if ver != 0x05 {
		client.Write([]byte{0x05, 0xFF}) // 不支持的版本
		return
	}
	client.Write([]byte{0x05, 0x00}) // 无需认证

	// 2. 解析 SOCKS5 请求
	buf := make([]byte, 4)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return
	}

	if buf[1] != 0x01 { // 只支持 CONNECT
		client.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 不支持的命令
		return
	}

	atyp := buf[3]

	// 解析目标地址
	var addr string
	switch atyp {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		addr = net.IP(ip).String()
	case 0x03: // 域名
		length, err := reader.ReadByte()
		if err != nil {
			return
		}
		host := make([]byte, length)
		if _, err := io.ReadFull(reader, host); err != nil {
			return
		}
		addr = string(host)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(reader, ip); err != nil {
			return
		}
		addr = net.IP(ip).String()
	default:
		client.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 不支持的地址类型
		return
	}

	// 解析端口
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	dest := fmt.Sprintf("%s:%d", addr, port)

	logger.Log.Info("SOCKS5 request", "target", dest, "client", client.RemoteAddr())

	// 3. 连接远程服务器
	server, err := net.DialTimeout("tcp", cfg.Server, cfg.GetTimeout())
	if err != nil {
		logger.Log.Error("Failed to connect to server", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 一般错误
		return
	}
	defer server.Close()

	// 设置服务器连接超时
	if cfg.Timeout > 0 {
		server.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	logger.Log.Debug("Connected to server", "server", cfg.Server)

	// 4. 执行握手认证
	salt, err := protocol.ClientHandshake(server, cfg.Password)
	if err != nil {
		logger.Log.Error("Handshake failed", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	logger.Log.Debug("Handshake successful")

	// 5. 创建加密器
	cipherInstance, err := cipher.NewCipher(cfg.Password, salt)
	if err != nil {
		logger.Log.Error("Failed to create cipher", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 6. 包装连接（加密 + 可选混淆）
	var serverReader io.Reader = server
	var serverWriter io.Writer = server

	if cfg.Obfuscate {
		serverReader = protocol.NewObfuscatedReader(serverReader)
		serverWriter = protocol.NewObfuscatedWriter(serverWriter)
	}

	secureReader := cipher.NewSecureReader(serverReader, cipherInstance)
	secureWriter := cipher.NewSecureWriter(serverWriter, cipherInstance)

	// 7. 发送目标地址到服务器
	// 协议: [地址长度(1字节)][地址字符串]
	if _, err := secureWriter.Write([]byte{byte(len(dest))}); err != nil {
		logger.Log.Error("Failed to send target address length", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := secureWriter.Write([]byte(dest)); err != nil {
		logger.Log.Error("Failed to send target address", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 8. 等待服务器连接目标的响应
	status := make([]byte, 1)
	if _, err := secureReader.Read(status); err != nil {
		logger.Log.Error("Failed to read server response", "error", err)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	if status[0] != 0 {
		logger.Log.Warn("Server failed to connect to target", "target", dest)
		client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// 清除超时，允许长时间数据传输
	client.SetDeadline(time.Time{})
	server.SetDeadline(time.Time{})

	// 9. 回复 SOCKS5 成功
	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	logger.Log.Debug("Tunnel established", "target", dest)

	// 10. 双向转发数据
	errCh := make(chan error, 2)

	// 浏览器 -> 服务器
	go func() {
		_, err := io.Copy(secureWriter, reader)
		errCh <- err
	}()

	// 服务器 -> 浏览器
	go func() {
		_, err := io.Copy(client, secureReader)
		errCh <- err
	}()

	// 等待任一方向结束
	err = <-errCh
	if err != nil && err != io.EOF {
		logger.Log.Debug("Transfer ended", "error", err)
	}

	logger.Log.Debug("Connection closed", "target", dest)
}
