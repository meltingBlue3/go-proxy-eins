package httpproxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go-proxy-eins/internal/cipher"
	"go-proxy-eins/internal/config"
	"go-proxy-eins/internal/logger"
	"go-proxy-eins/internal/protocol"
)

// HandleHTTPConnect 处理 HTTP CONNECT 请求
// requestLine: 第一行请求，如 "CONNECT github.com:443 HTTP/1.1\r\n"
func HandleHTTPConnect(client net.Conn, reader *bufio.Reader, requestLine string, cfg *config.LocalConfig) {
	defer client.Close()

	// 设置超时
	if cfg.Timeout > 0 {
		client.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	logger.Log.Debug("New HTTP CONNECT request", "remote", client.RemoteAddr())

	// 解析 CONNECT 请求
	// 格式: "CONNECT host:port HTTP/1.1"
	parts := strings.Fields(requestLine)
	if len(parts) < 2 {
		sendHTTPError(client, 400, "Bad Request")
		return
	}

	targetAddr := parts[1]
	logger.Log.Info("HTTP CONNECT request", "target", targetAddr, "client", client.RemoteAddr())

	// 读取并丢弃剩余的 HTTP 头（使用传入的 reader）
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		// 空行表示头部结束
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// 连接到远程服务器
	server, err := net.DialTimeout("tcp", cfg.Server, cfg.GetTimeout())
	if err != nil {
		logger.Log.Error("Failed to connect to server", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}
	defer server.Close()

	// 设置服务器连接超时
	if cfg.Timeout > 0 {
		server.SetDeadline(time.Now().Add(cfg.GetTimeout()))
	}

	logger.Log.Debug("Connected to server", "server", cfg.Server)

	// 执行握手认证
	salt, err := protocol.ClientHandshake(server, cfg.Password)
	if err != nil {
		logger.Log.Error("Handshake failed", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}

	logger.Log.Debug("Handshake successful")

	// 创建加密器
	cipherInstance, err := cipher.NewCipher(cfg.Password, salt)
	if err != nil {
		logger.Log.Error("Failed to create cipher", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}

	// 包装连接（加密 + 可选混淆）
	var serverReader io.Reader = server
	var serverWriter io.Writer = server

	if cfg.Obfuscate {
		serverReader = protocol.NewObfuscatedReader(serverReader)
		serverWriter = protocol.NewObfuscatedWriter(serverWriter)
	}

	secureReader := cipher.NewSecureReader(serverReader, cipherInstance)
	secureWriter := cipher.NewSecureWriter(serverWriter, cipherInstance)

	// 发送目标地址到服务器
	// 协议: [地址长度(1字节)][地址字符串]
	if _, err := secureWriter.Write([]byte{byte(len(targetAddr))}); err != nil {
		logger.Log.Error("Failed to send target address length", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}
	if _, err := secureWriter.Write([]byte(targetAddr)); err != nil {
		logger.Log.Error("Failed to send target address", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}

	// 等待服务器连接目标的响应
	status := make([]byte, 1)
	if _, err := secureReader.Read(status); err != nil {
		logger.Log.Error("Failed to read server response", "error", err)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}

	if status[0] != 0 {
		logger.Log.Warn("Server failed to connect to target", "target", targetAddr)
		sendHTTPError(client, 502, "Bad Gateway")
		return
	}

	// 清除超时，允许长时间数据传输
	client.SetDeadline(time.Time{})
	server.SetDeadline(time.Time{})

	// 发送 HTTP 200 Connection Established 响应
	response := "HTTP/1.1 200 Connection Established\r\n\r\n"
	if _, err := client.Write([]byte(response)); err != nil {
		logger.Log.Error("Failed to send HTTP response", "error", err)
		return
	}

	logger.Log.Debug("HTTP tunnel established", "target", targetAddr)

	// 双向转发数据
	errCh := make(chan error, 2)

	// 浏览器 -> 服务器
	go func() {
		_, err := io.Copy(secureWriter, client)
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

	logger.Log.Debug("HTTP connection closed", "target", targetAddr)
}

// sendHTTPError 发送 HTTP 错误响应
func sendHTTPError(conn net.Conn, statusCode int, statusText string) {
	response := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", statusCode, statusText)
	conn.Write([]byte(response))
}

// ParseHTTPRequest 解析 HTTP 请求第一行
func ParseHTTPRequest(requestLine string) (method, target, version string, err error) {
	parts := strings.Fields(strings.TrimSpace(requestLine))
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid HTTP request line: %s", requestLine)
	}
	return parts[0], parts[1], parts[2], nil
}

// IsHTTPConnect 检查是否是 HTTP CONNECT 请求
func IsHTTPConnect(data []byte) bool {
	if len(data) < 7 {
		return false
	}
	return string(data[:7]) == "CONNECT"
}

// DetectProtocol 检测协议类型 (HTTP CONNECT 或 SOCKS5)
func DetectProtocol(reader *bufio.Reader) (isHTTP bool, firstData []byte, err error) {
	// 尝试窥探第一个字节
	firstByte, err := reader.Peek(1)
	if err != nil {
		return false, nil, err
	}

	// SOCKS5 的第一个字节是版本号 0x05
	if firstByte[0] == 0x05 {
		return false, firstByte, nil
	}

	// 尝试读取第一行以检查是否是 HTTP
	// 窥探更多字节以检查是否包含 "CONNECT"
	peek, err := reader.Peek(8)
	if err != nil && err != io.EOF {
		return false, nil, err
	}

	if len(peek) >= 7 && string(peek[:7]) == "CONNECT" {
		return true, peek, nil
	}

	// 默认视为 SOCKS5
	return false, firstByte, nil
}

// ReadHTTPRequestLine 读取 HTTP 请求第一行
func ReadHTTPRequestLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ValidateHTTPMethod 验证 HTTP 方法
func ValidateHTTPMethod(method string) bool {
	validMethods := map[string]bool{
		http.MethodConnect: true,
		http.MethodGet:     true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodDelete:  true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		http.MethodPatch:   true,
		http.MethodTrace:   true,
	}
	return validMethods[strings.ToUpper(method)]
}
