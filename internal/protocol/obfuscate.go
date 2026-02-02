package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// 最大填充长度
	MaxPaddingLen = 64
)

// ObfuscatedReader 包装 io.Reader，自动去除混淆
type ObfuscatedReader struct {
	src io.Reader
}

// NewObfuscatedReader 创建混淆读取器
func NewObfuscatedReader(src io.Reader) *ObfuscatedReader {
	return &ObfuscatedReader{src: src}
}

// Read 实现 io.Reader，自动去除填充
func (or *ObfuscatedReader) Read(p []byte) (n int, err error) {
	// 读取前填充长度 (1 字节)
	lenBuf := make([]byte, 1)
	if _, err := io.ReadFull(or.src, lenBuf); err != nil {
		return 0, err
	}
	prePaddingLen := int(lenBuf[0])

	if prePaddingLen > MaxPaddingLen {
		return 0, fmt.Errorf("invalid padding length: %d", prePaddingLen)
	}

	// 跳过前填充
	if prePaddingLen > 0 {
		padding := make([]byte, prePaddingLen)
		if _, err := io.ReadFull(or.src, padding); err != nil {
			return 0, err
		}
	}

	// 读取实际数据长度 (2 字节)
	if _, err := io.ReadFull(or.src, lenBuf[:1]); err != nil {
		return 0, err
	}
	var dataLenBuf [2]byte
	dataLenBuf[0] = lenBuf[0]
	if _, err := io.ReadFull(or.src, dataLenBuf[1:]); err != nil {
		return 0, err
	}
	dataLen := binary.BigEndian.Uint16(dataLenBuf[:])

	// 读取实际数据
	if int(dataLen) > len(p) {
		return 0, fmt.Errorf("buffer too small: need %d, have %d", dataLen, len(p))
	}

	data := make([]byte, dataLen)
	if _, err := io.ReadFull(or.src, data); err != nil {
		return 0, err
	}

	// 读取后填充长度
	if _, err := io.ReadFull(or.src, lenBuf); err != nil {
		return 0, err
	}
	postPaddingLen := int(lenBuf[0])

	if postPaddingLen > MaxPaddingLen {
		return 0, fmt.Errorf("invalid post padding length: %d", postPaddingLen)
	}

	// 跳过后填充
	if postPaddingLen > 0 {
		padding := make([]byte, postPaddingLen)
		if _, err := io.ReadFull(or.src, padding); err != nil {
			return 0, err
		}
	}

	// 复制到输出
	n = copy(p, data)
	return n, nil
}

// ObfuscatedWriter 包装 io.Writer，自动添加混淆
type ObfuscatedWriter struct {
	dst io.Writer
}

// NewObfuscatedWriter 创建混淆写入器
func NewObfuscatedWriter(dst io.Writer) *ObfuscatedWriter {
	return &ObfuscatedWriter{dst: dst}
}

// Write 实现 io.Writer，自动添加填充
func (ow *ObfuscatedWriter) Write(p []byte) (n int, err error) {
	// 生成随机填充长度
	prePaddingLen := randomPaddingLen()
	postPaddingLen := randomPaddingLen()

	// 生成填充数据
	prePadding := make([]byte, prePaddingLen)
	postPadding := make([]byte, postPaddingLen)
	if prePaddingLen > 0 {
		rand.Read(prePadding)
	}
	if postPaddingLen > 0 {
		rand.Read(postPadding)
	}

	// 写入：[前填充长度(1)][前填充][数据长度(2)][数据][后填充长度(1)][后填充]
	
	// 前填充长度
	if _, err := ow.dst.Write([]byte{byte(prePaddingLen)}); err != nil {
		return 0, err
	}

	// 前填充
	if prePaddingLen > 0 {
		if _, err := ow.dst.Write(prePadding); err != nil {
			return 0, err
		}
	}

	// 数据长度
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(p)))
	if _, err := ow.dst.Write(lenBuf); err != nil {
		return 0, err
	}

	// 实际数据
	if _, err := ow.dst.Write(p); err != nil {
		return 0, err
	}

	// 后填充长度
	if _, err := ow.dst.Write([]byte{byte(postPaddingLen)}); err != nil {
		return 0, err
	}

	// 后填充
	if postPaddingLen > 0 {
		if _, err := ow.dst.Write(postPadding); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

// randomPaddingLen 生成随机填充长度 (0-MaxPaddingLen)
func randomPaddingLen() int {
	var b [1]byte
	rand.Read(b[:])
	return int(b[0]) % (MaxPaddingLen + 1)
}
