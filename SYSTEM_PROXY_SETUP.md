# Windows 系统代理自动配置功能

## 功能概述

客户端现在支持：
1. **双协议监听**：同时运行 SOCKS5 和 HTTP CONNECT 代理
2. **自动系统代理**：启动时自动配置 Windows 系统代理
3. **优雅退出**：退出时自动恢复原有代理设置

## 新增模块

### 1. `internal/sysproxy/` - Windows 系统代理管理
- `windows.go`: 代理配置的读取、设置和恢复
- `notify_windows.go`: Windows API 调用，通知系统代理变更

### 2. `internal/httpproxy/` - HTTP CONNECT 代理
- `handler.go`: HTTP CONNECT 请求处理器

## 配置变更

### 新增配置项

```json
{
  "local_addr": "127.0.0.1:1080",      // SOCKS5 监听地址
  "http_proxy_addr": "127.0.0.1:8080", // HTTP 代理监听地址（新增）
  "server": "your-server:8081",
  "password": "your-password",
  "timeout": 30,
  "log_level": "info",
  "obfuscate": true,
  "auto_proxy": true                   // 自动设置系统代理（新增）
}
```

### 配置说明

- `http_proxy_addr`: HTTP/CONNECT 代理监听地址，浏览器通过此地址访问代理
- `auto_proxy`: 是否自动设置系统代理
  - `true`: 启动时自动配置 Windows 系统代理指向 `http_proxy_addr`
  - `false`: 不自动配置，需要手动设置浏览器代理

## 使用方式

### 1. 启动客户端

```bash
# Windows
.\local.exe

# 或指定配置文件
.\local.exe -c local.config.json
```

### 2. 自动配置生效

启动后，程序会：
1. 备份当前系统代理设置
2. 将系统 HTTP 代理设置为 `127.0.0.1:8080`
3. 启动 SOCKS5 监听器（`127.0.0.1:1080`）
4. 启动 HTTP 代理监听器（`127.0.0.1:8080`）

### 3. 访问网站

打开浏览器（Chrome、Edge、Firefox 等），直接访问 GitHub 或其他网站：
- 浏览器会自动使用系统代理 `127.0.0.1:8080`
- HTTP CONNECT 请求被转发到加密隧道
- 数据通过远程服务器访问目标网站

### 4. 退出程序

按 `Ctrl+C` 退出，程序会自动恢复原有系统代理设置。

## 工作流程

```
浏览器
  ↓ (使用系统代理)
HTTP Proxy (127.0.0.1:8080)
  ↓ (解析 CONNECT 请求)
加密隧道
  ↓ (ChaCha20-Poly1305 + 可选混淆)
远程服务器 (your-server:8081)
  ↓
目标网站 (如 github.com)
```

## 架构优势

1. **兼容性**：HTTP 代理被所有主流浏览器和应用支持
2. **透明性**：无需手动配置浏览器
3. **安全性**：保留原有的加密和混淆机制
4. **灵活性**：
   - SOCKS5 端口仍可用于其他应用
   - 可通过 `auto_proxy: false` 禁用自动配置

## 命令行参数

```bash
# 禁用自动代理
.\local.exe -auto-proxy=false

# 自定义 HTTP 代理地址
.\local.exe -http 127.0.0.1:9090

# 自定义 SOCKS5 地址
.\local.exe -b 127.0.0.1:1081
```

## 故障排查

### 问题：浏览器无法连接

1. 检查客户端是否正常运行
2. 查看 Windows 代理设置：
   - 设置 → 网络和 Internet → 代理
   - 确认 HTTP 代理为 `127.0.0.1:8080`
3. 检查防火墙是否阻止连接

### 问题：退出后代理未恢复

手动恢复：
1. 打开 Windows 设置 → 网络和 Internet → 代理
2. 关闭"使用代理服务器"

或运行 PowerShell：
```powershell
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyEnable -Value 0
```

### 问题：系统代理设置失败

- 需要管理员权限修改注册表
- 尝试以管理员身份运行：右键 `local.exe` → 以管理员身份运行

## 技术细节

### HTTP CONNECT 协议

客户端实现标准的 HTTP CONNECT 方法：

```
客户端请求：
CONNECT github.com:443 HTTP/1.1
Host: github.com:443

服务器响应：
HTTP/1.1 200 Connection Established

[开始双向透明传输]
```

### Windows 注册表操作

修改的注册表项：
```
HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Internet Settings
- ProxyEnable (DWORD): 1
- ProxyServer (STRING): 127.0.0.1:8080
- ProxyOverride (STRING): localhost;127.*;...
```

### 系统通知

使用 WinINet API 通知系统刷新代理设置：
- `InternetSetOptionW(INTERNET_OPTION_SETTINGS_CHANGED)`
- `InternetSetOptionW(INTERNET_OPTION_REFRESH)`

## 安全考虑

1. **本地绑定**：默认只监听 `127.0.0.1`，不暴露到外网
2. **代理排除**：本地地址不走代理（`localhost`, `127.*`, `192.168.*` 等）
3. **加密不变**：仍使用 ChaCha20-Poly1305 加密所有流量
4. **混淆可选**：保留流量混淆功能对抗 DPI

## 未来改进

- [ ] 支持 PAC (Proxy Auto-Configuration) 文件
- [ ] 支持规则路由（部分域名走代理）
- [ ] 支持 macOS 和 Linux 系统代理设置
- [ ] 添加系统托盘图标和 GUI
- [ ] 支持更多 HTTP 方法（GET、POST 等）用于完整 HTTP 代理
