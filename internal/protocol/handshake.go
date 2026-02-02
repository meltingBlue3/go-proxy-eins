package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

const (
	// 握手参数
	SaltLen       = 32
	TimestampLen  = 8
	HMACLen       = 32
	HandshakeLen  = SaltLen + TimestampLen + HMACLen
	
	// 时间戳允许误差（秒）
	TimeSkewAllowance = 30
)

// ClientHandshake 客户端执行握手
// 发送: [salt(32)][timestamp(8)][HMAC(32)]
func ClientHandshake(conn io.ReadWriter, password string) ([]byte, error) {
	// 生成随机 salt
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// 当前时间戳（Unix 秒）
	timestamp := time.Now().Unix()
	timestampBytes := make([]byte, TimestampLen)
	binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))

	// 计算 HMAC: HMAC-SHA256(password, salt + timestamp)
	h := hmac.New(sha256.New, []byte(password))
	h.Write(salt)
	h.Write(timestampBytes)
	mac := h.Sum(nil)

	// 发送握手数据
	handshake := make([]byte, 0, HandshakeLen)
	handshake = append(handshake, salt...)
	handshake = append(handshake, timestampBytes...)
	handshake = append(handshake, mac...)

	if _, err := conn.Write(handshake); err != nil {
		return nil, fmt.Errorf("failed to send handshake: %w", err)
	}

	// 读取服务端响应 (1 字节)
	response := make([]byte, 1)
	if _, err := io.ReadFull(conn, response); err != nil {
		return nil, fmt.Errorf("failed to read handshake response: %w", err)
	}

	if response[0] != 0 {
		return nil, fmt.Errorf("authentication failed")
	}

	return salt, nil
}

// ServerHandshake 服务端执行握手验证
// 返回 salt 用于后续加密
func ServerHandshake(conn io.Reader, writer io.Writer, password string) ([]byte, error) {
	// 读取握手数据
	handshake := make([]byte, HandshakeLen)
	if _, err := io.ReadFull(conn, handshake); err != nil {
		return nil, fmt.Errorf("failed to read handshake: %w", err)
	}

	// 解析握手数据
	salt := handshake[:SaltLen]
	timestampBytes := handshake[SaltLen : SaltLen+TimestampLen]
	receivedMAC := handshake[SaltLen+TimestampLen:]

	// 验证时间戳
	timestamp := int64(binary.BigEndian.Uint64(timestampBytes))
	now := time.Now().Unix()
	if abs(now-timestamp) > TimeSkewAllowance {
		writer.Write([]byte{1}) // 认证失败
		return nil, fmt.Errorf("timestamp out of range: %d vs %d", timestamp, now)
	}

	// 验证 HMAC
	h := hmac.New(sha256.New, []byte(password))
	h.Write(salt)
	h.Write(timestampBytes)
	expectedMAC := h.Sum(nil)

	if !hmac.Equal(receivedMAC, expectedMAC) {
		writer.Write([]byte{1}) // 认证失败
		return nil, fmt.Errorf("invalid authentication")
	}

	// 认证成功
	if _, err := writer.Write([]byte{0}); err != nil {
		return nil, fmt.Errorf("failed to send success response: %w", err)
	}

	return salt, nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
