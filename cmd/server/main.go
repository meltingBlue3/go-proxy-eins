package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"go-proxy-eins/internal/cipher"
	"go-proxy-eins/internal/config"
	"go-proxy-eins/internal/logger"
	"go-proxy-eins/internal/protocol"
	"go-proxy-eins/internal/socks5"
)

func main() {
	// 加载配置
	cfg, err := config.LoadServerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	logger.Init(logger.ParseLevel(cfg.LogLevel), os.Stdout)
	logger.Log.Info("Starting proxy server", "port", cfg.Port, "obfuscate", cfg.Obfuscate)

	// 监听端口
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", cfg.Port))
	if err != nil {
		logger.Log.Error("Failed to listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Log.Info("Server is running", "address", listener.Addr())

	// 接受连接
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Log.Warn("Failed to accept connection", "error", err)
			continue
		}

		go handleConnection(conn, cfg)
	}
}

func handleConnection(conn net.Conn, cfg *config.ServerConfig) {
	defer conn.Close()

	// 设置超时
	if cfg.Timeout > 0 {
		conn.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	logger.Log.Debug("New connection", "remote", conn.RemoteAddr())

	// 1. 握手认证
	salt, err := protocol.ServerHandshake(conn, conn, cfg.Password)
	if err != nil {
		logger.Log.Warn("Handshake failed", "remote", conn.RemoteAddr(), "error", err)
		return
	}

	logger.Log.Debug("Handshake successful", "remote", conn.RemoteAddr())

	// 2. 创建加密器
	cipherInstance, err := cipher.NewCipher(cfg.Password, salt)
	if err != nil {
		logger.Log.Error("Failed to create cipher", "error", err)
		return
	}

	// 3. 包装连接（加密 + 可选混淆）
	var reader io.Reader = conn
	var writer io.Writer = conn

	if cfg.Obfuscate {
		reader = protocol.NewObfuscatedReader(reader)
		writer = protocol.NewObfuscatedWriter(writer)
	}

	secureReader := cipher.NewSecureReader(reader, cipherInstance)
	secureWriter := cipher.NewSecureWriter(writer, cipherInstance)

	// 4. 读取目标地址
	// 协议: [地址长度(1字节)][地址字符串]
	lenBuf := make([]byte, 1)
	if _, err := secureReader.Read(lenBuf); err != nil {
		logger.Log.Error("Failed to read target address length", "error", err)
		return
	}
	addrLen := int(lenBuf[0])

	addrBuf := make([]byte, addrLen)
	if _, err := io.ReadFull(secureReader, addrBuf); err != nil {
		logger.Log.Error("Failed to read target address", "error", err)
		return
	}
	targetAddr := string(addrBuf)

	logger.Log.Info("Connecting to target", "target", targetAddr, "client", conn.RemoteAddr())

	// 5. 连接目标服务器（通过上游 SOCKS5 代理或直连）
	var target net.Conn
	if cfg.HasUpstreamProxy() {
		// 通过上游 SOCKS5 代理连接
		logger.Log.Debug("Using upstream SOCKS5 proxy", "proxy", cfg.UpstreamProxy, "target", targetAddr)
		target, err = socks5.DialWithAuth(
			cfg.UpstreamProxy,
			targetAddr,
			cfg.UpstreamUsername,
			cfg.UpstreamPassword,
			cfg.GetTimeout(),
		)
		if err != nil {
			logger.Log.Warn("Failed to connect via upstream proxy", "proxy", cfg.UpstreamProxy, "target", targetAddr, "error", err)
			secureWriter.Write([]byte{1}) // 连接失败
			return
		}
	} else {
		// 直接连接目标
		target, err = net.DialTimeout("tcp", targetAddr, cfg.GetTimeout())
		if err != nil {
			logger.Log.Warn("Failed to connect to target", "target", targetAddr, "error", err)
			secureWriter.Write([]byte{1}) // 连接失败
			return
		}
	}
	defer target.Close()

	// 清除超时，允许长时间数据传输
	conn.SetDeadline(time.Time{})
	target.SetDeadline(time.Time{})

	// 6. 通知客户端连接成功
	if _, err := secureWriter.Write([]byte{0}); err != nil {
		logger.Log.Error("Failed to send success response", "error", err)
		return
	}

	logger.Log.Debug("Connection established", "target", targetAddr)

	// 7. 双向转发数据
	errCh := make(chan error, 2)

	// 客户端 -> 目标
	go func() {
		_, err := io.Copy(target, secureReader)
		errCh <- err
	}()

	// 目标 -> 客户端
	go func() {
		_, err := io.Copy(secureWriter, target)
		errCh <- err
	}()

	// 等待任一方向结束
	err = <-errCh
	if err != nil && err != io.EOF {
		logger.Log.Debug("Transfer ended", "error", err)
	}

	logger.Log.Debug("Connection closed", "target", targetAddr)
}
