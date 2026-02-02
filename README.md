# Go Proxy Eins

一个安全的 SOCKS5 代理工具，使用 ChaCha20-Poly1305 加密算法和 Argon2 密钥派生。

## 特性

- **强加密**: 使用 ChaCha20-Poly1305 AEAD 加密，提供认证加密
- **安全认证**: HMAC-SHA256 握手协议，防止中间人攻击
- **密钥派生**: Argon2id 从密码派生加密密钥
- **流量混淆**: 可选的随机填充混淆，使流量特征更难识别
- **灵活配置**: 支持 JSON 配置文件和命令行参数
- **日志控制**: 多级别日志输出 (debug/info/warn/error)
- **超时管理**: 可配置的连接超时

## 架构

```
[浏览器] <-> [Local SOCKS5] <-> [加密+混淆] <-> [Server] <-> [目标网站]
```

## 安装

### 前提条件

- Go 1.19 或更高版本

### 构建

```bash
# 构建服务端
go build -o server.exe ./cmd/server

# 构建客户端
go build -o local.exe ./cmd/local
```

如果网络连接受限，无法下载依赖，可以手动添加到 `go.mod`:

```bash
go mod edit -require golang.org/x/crypto@latest
go mod tidy
```

## 使用

### 1. 服务端部署

在远程服务器上运行：

```bash
# 使用命令行参数
./server -p 8081 -k "your-strong-password" -l info -o

# 或使用配置文件
./server -c server.config.json
```

**服务端参数**:
- `-c`: 配置文件路径
- `-p`: 监听端口 (默认: 8081)
- `-k`: 加密密码 (必需)
- `-t`: 连接超时秒数 (默认: 30)
- `-l`: 日志级别 debug/info/warn/error (默认: info)
- `-o`: 启用流量混淆

**配置文件示例** (`server.config.json`):
```json
{
  "port": 8081,
  "password": "your-strong-password",
  "timeout": 30,
  "log_level": "info",
  "obfuscate": true
}
```

### 2. 本地客户端

在本地机器上运行：

```bash
# 使用命令行参数
./local -s "your-server.com:8081" -k "your-strong-password" -l info -o

# 或使用配置文件
./local -c local.config.json
```

**客户端参数**:
- `-c`: 配置文件路径
- `-b`: 本地监听地址 (默认: 127.0.0.1:1080)
- `-s`: 服务器地址 (必需)
- `-k`: 加密密码 (必需)
- `-t`: 连接超时秒数 (默认: 30)
- `-l`: 日志级别 debug/info/warn/error (默认: info)
- `-o`: 启用流量混淆

**配置文件示例** (`local.config.json`):
```json
{
  "local_addr": "127.0.0.1:1080",
  "server": "your-server.com:8081",
  "password": "your-strong-password",
  "timeout": 30,
  "log_level": "info",
  "obfuscate": true
}
```

### 3. 浏览器配置

配置浏览器使用 SOCKS5 代理:
- 代理地址: `127.0.0.1`
- 端口: `1080`
- 类型: SOCKS5

或使用系统代理设置。

## 安全性

### 加密协议

1. **握手阶段**:
   - 客户端生成 32 字节随机 salt
   - 发送 `[salt][timestamp][HMAC(password, salt+timestamp)]`
   - 服务端验证 HMAC 和时间戳（允许 30 秒误差）

2. **数据传输**:
   - 使用 Argon2id 从密码和 salt 派生 32 字节密钥
   - ChaCha20-Poly1305 加密每个数据包
   - 每个包使用递增的 nonce（防重放）
   - 数据格式: `[长度(2字节)][nonce(24字节)][加密数据+认证标签]`

3. **流量混淆** (可选):
   - 在数据包前后添加 0-64 字节随机填充
   - 模糊真实流量长度特征

### 密码建议

- 使用至少 16 个字符的强密码
- 包含大小写字母、数字和特殊符号
- 客户端和服务端密码必须完全一致
- 定期更换密码

### 注意事项

- **不要**在不安全的通道传输密码
- **不要**使用弱密码或默认密码
- **建议**启用流量混淆 (`-o` 参数)
- **建议**定期检查日志，监控异常连接
- **建议**在生产环境关闭 debug 日志

## 性能优化

- ChaCha20-Poly1305 在没有 AES 硬件加速的平台上性能优异
- Argon2 密钥派生使用优化参数，平衡安全性和性能
- 流量混淆会增加约 10-20% 的带宽开销

## 故障排查

### 连接失败

1. 检查服务器地址和端口是否正确
2. 确认防火墙已开放相应端口
3. 验证客户端和服务端密码是否一致
4. 检查网络连接是否正常

### 认证失败

- 确保密码完全相同（包括大小写）
- 检查服务器和客户端时间是否同步（误差不超过 30 秒）

### 性能问题

- 增加超时时间 (`-t` 参数)
- 检查网络延迟和带宽
- 考虑关闭流量混淆以提升性能

### 调试模式

启用 debug 日志查看详细信息:

```bash
./local -l debug
./server -l debug
```

## 项目结构

```
go-proxy-eins/
├── cmd/
│   ├── local/          # 本地客户端
│   └── server/         # 远程服务端
├── internal/
│   ├── cipher/         # ChaCha20-Poly1305 加密
│   ├── config/         # 配置管理
│   ├── logger/         # 日志系统
│   └── protocol/       # 握手和混淆协议
├── *.config.json       # 配置文件示例
└── README.md
```

## 技术栈

- **语言**: Go 1.19+
- **加密库**: golang.org/x/crypto
- **加密算法**: ChaCha20-Poly1305
- **密钥派生**: Argon2id
- **认证**: HMAC-SHA256
- **日志**: log/slog 标准库

## License

MIT

## 免责声明

本工具仅供学习和研究使用。使用者需遵守当地法律法规，作者不对使用本工具产生的任何后果负责。
