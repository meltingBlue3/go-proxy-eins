package cipher

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// Argon2 参数
	Argon2Time    = 1
	Argon2Memory  = 64 * 1024
	Argon2Threads = 4
	Argon2KeyLen  = 32

	// Salt 长度
	SaltLen = 32

	// 数据包最大长度 (16MB)
	MaxPacketSize = 0xFFFF
)

// Cipher 封装 ChaCha20-Poly1305 AEAD 加密
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher 从密码和 salt 创建加密器
func NewCipher(password string, salt []byte) (*Cipher, error) {
	if len(salt) != SaltLen {
		return nil, fmt.Errorf("invalid salt length: %d, expected %d", len(salt), SaltLen)
	}

	// 使用 Argon2id 从密码派生密钥
	key := argon2.IDKey(
		[]byte(password),
		salt,
		Argon2Time,
		Argon2Memory,
		Argon2Threads,
		Argon2KeyLen,
	)

	// 创建 ChaCha20-Poly1305 AEAD
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	return &Cipher{aead: aead}, nil
}

// SecureReader 包装 io.Reader，自动解密数据
type SecureReader struct {
	src    io.Reader
	cipher *Cipher
	nonce  uint64
	buffer []byte
}

// NewSecureReader 创建安全读取器
func NewSecureReader(src io.Reader, cipher *Cipher) *SecureReader {
	return &SecureReader{
		src:    src,
		cipher: cipher,
		nonce:  0,
		buffer: make([]byte, 0, MaxPacketSize),
	}
}

// Read 实现 io.Reader，自动解密读取的数据
func (sr *SecureReader) Read(p []byte) (n int, err error) {
	// 读取数据长度 (2 字节)
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(sr.src, lenBuf); err != nil {
		return 0, err
	}
	dataLen := binary.BigEndian.Uint16(lenBuf)

	if dataLen > MaxPacketSize {
		return 0, fmt.Errorf("packet too large: %d", dataLen)
	}

	// 读取 nonce
	nonceBytes := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(sr.src, nonceBytes); err != nil {
		return 0, err
	}

	// 读取加密数据
	encryptedData := make([]byte, dataLen)
	if _, err := io.ReadFull(sr.src, encryptedData); err != nil {
		return 0, err
	}

	// 解密数据
	plaintext, err := sr.cipher.aead.Open(nil, nonceBytes, encryptedData, nil)
	if err != nil {
		return 0, fmt.Errorf("decryption failed: %w", err)
	}

	// 复制到输出缓冲区
	n = copy(p, plaintext)
	return n, nil
}

// SecureWriter 包装 io.Writer，自动加密数据
type SecureWriter struct {
	dst    io.Writer
	cipher *Cipher
	nonce  uint64
}

// NewSecureWriter 创建安全写入器
func NewSecureWriter(dst io.Writer, cipher *Cipher) *SecureWriter {
	return &SecureWriter{
		dst:    dst,
		cipher: cipher,
		nonce:  0,
	}
}

// Write 实现 io.Writer，自动加密写入的数据
func (sw *SecureWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 限制单个数据包大小
	if len(p) > MaxPacketSize-sw.cipher.aead.Overhead() {
		return 0, fmt.Errorf("data too large: %d", len(p))
	}

	// 生成 nonce（使用计数器）
	nonceBytes := make([]byte, chacha20poly1305.NonceSizeX)
	binary.BigEndian.PutUint64(nonceBytes[16:], sw.nonce)
	sw.nonce++

	// 加密数据
	ciphertext := sw.cipher.aead.Seal(nil, nonceBytes, p, nil)

	// 写入：[2字节长度][nonce][加密数据]
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(ciphertext)))

	if _, err := sw.dst.Write(lenBuf); err != nil {
		return 0, err
	}

	if _, err := sw.dst.Write(nonceBytes); err != nil {
		return 0, err
	}

	if _, err := sw.dst.Write(ciphertext); err != nil {
		return 0, err
	}

	return len(p), nil
}

// GenerateSalt 生成随机 salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}
