# Security Policy

## 报告安全漏洞

如果你发现安全漏洞，请**不要**通过公开 Issue 报告。

请通过 Issue 报告（标记为 confidential）。

我们会尽快回复并处理。

## Supported Versions

| 版本 | 支持状态 |
|------|---------|
| 最新 release | ✅ |
| 旧版本 | ❌ 请升级到最新版 |

## 安全机制

Goink 内置以下安全措施：

- **双层沙箱**：正则白名单 + SafePath 防路径穿越
- **Bearer Token 认证**：HTTP API 需要 Token
- **自动 HTTPS**：移动端通信自动加密
- **文件写入保护**：编辑前重读对比，防止覆盖手动修改

---

# 安全政策

## 报告安全漏洞

If you discover a security vulnerability, please **do NOT** report it through public Issues.

Report via Issue (mark as confidential).

We will respond as soon as possible.

## Security Measures

Goink includes:

- **Dual-layer sandbox**: regex whitelist + SafePath path traversal protection
- **Bearer Token auth**: HTTP API requires token
- **Auto HTTPS**: mobile communication encrypted automatically
- **Write protection**: re-read before edit to prevent overwriting manual changes
