# 安装指南

## 前提条件

- Go 1.19 或更高版本
- 网络连接（用于下载依赖）

## 快速开始

### Windows

```powershell
# 使用构建脚本（推荐）
.\build.ps1

# 或手动构建
go build -o server.exe .\cmd\server
go build -o local.exe .\cmd\local
```

### Linux / macOS

```bash
# 使用构建脚本（推荐）
chmod +x build.sh
./build.sh

# 或手动构建
go build -o server ./cmd/server
go build -o local ./cmd/local
```

## 网络受限环境

如果无法连接到 `proxy.golang.org`，可以使用国内镜像：

### 方法 1：设置 GOPROXY 环境变量

```bash
# Windows PowerShell
$env:GOPROXY = "https://goproxy.cn,direct"
go mod download
go build -o server.exe .\cmd\server
go build -o local.exe .\cmd\local

# Linux / macOS
export GOPROXY=https://goproxy.cn,direct
go mod download
go build -o server ./cmd/server
go build -o local ./cmd/local
```

### 方法 2：手动下载依赖

如果以上方法都不可行，可以从其他机器下载依赖后复制：

1. 在有网络的机器上运行：
   ```bash
   go mod download
   go mod vendor  # 创建 vendor 目录
   ```

2. 复制整个项目目录（包括 `vendor` 目录）到目标机器

3. 在目标机器上构建：
   ```bash
   go build -mod=vendor -o server ./cmd/server
   go build -mod=vendor -o local ./cmd/local
   ```

## 依赖说明

本项目仅依赖一个外部库：

- `golang.org/x/crypto` - 提供 ChaCha20-Poly1305 和 Argon2 加密算法

## 验证安装

构建成功后，可以测试程序是否正常：

```bash
# Windows
.\server.exe -h
.\local.exe -h

# Linux / macOS
./server -h
./local -h
```

应该看到帮助信息输出。

## 常见问题

### 问题 1: 网络连接超时

**错误信息**:
```
dial tcp: connectex: A connection attempt failed...
```

**解决方法**: 使用国内 GOPROXY 镜像（见上文）

### 问题 2: Go 版本过低

**错误信息**:
```
go: module requires Go 1.19 or higher
```

**解决方法**: 升级 Go 到 1.19 或更高版本
- 下载地址: https://golang.org/dl/

### 问题 3: 缺少 go.sum

**错误信息**:
```
missing go.sum entry for module...
```

**解决方法**: 运行 `go mod tidy` 生成 go.sum 文件

### 问题 4: 找不到包

**错误信息**:
```
cannot find package "golang.org/x/crypto/..."
```

**解决方法**: 
```bash
go get golang.org/x/crypto
go mod tidy
```

## 下一步

安装完成后，请参考 [README.md](README.md) 了解如何配置和使用。
