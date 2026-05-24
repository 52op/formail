# Formail SSO 配置指南

Formail 支持两种认证模式，通过 `config.toml` 的 `[security]` 段切换：

- `standalone`（默认）：使用本地 JWT（HS256），独立管理账号
- `sso`：接入 GoAuth 统一认证，验证 GoAuth 颁发的 RS256 JWT

---

## 配置步骤

### 1. 获取 GoAuth 公钥

```bash
curl https://auth.it0731.cn/api/public-key
```

### 2. 修改 config.toml

打开 `config.toml`，在 `[security]` 段添加或修改以下字段：

```toml
[security]
# 独立模式的 JWT 密钥（SSO 模式下仍需保留，不会被读取）
jwt_secret = "your-existing-secret"
encryption_key = "your-existing-encryption-key"

# SSO 配置
auth_mode = "sso"
sso_issuer = "https://auth.it0731.cn"
sso_cookie_name = "_goauth_token"
# 公钥 PEM，多行用 \n 连接写成一行
sso_public_key = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...\n-----END PUBLIC KEY-----"
```

> **公钥换行**：TOML 单行字符串中换行必须写成 `\n`，不能直接换行。可用以下命令快速处理：
> ```bash
> curl https://auth.it0731.cn/api/public-key | awk 'NR>1{printf "%s\\n", prev} {prev=$0} END{print prev}'
> ```

### 3. 重启 Formail

```bash
./formail
```

---

## SSO 认证流程

Formail 是纯 API 服务（无登录页面），SSO 模式下的认证流程如下：

```
客户端请求携带 Authorization: Bearer <JWT>
    或
客户端请求携带 Cookie: _goauth_token=<JWT>
    ↓
Formail 中间件用 GoAuth 公钥验证 RS256 JWT
    ↓
提取 user_id、username、role 注入请求上下文
    ↓
业务逻辑处理
```

JWT 来源优先级：**Authorization Bearer header > Cookie**

---

## Cookie 共享（推荐）

若 GoAuth 配置了 `cookie_domain = ".it0731.cn"`，则用户在 GoAuth 登录后，访问 `form.it0731.cn` 时浏览器会自动携带 `_goauth_token` Cookie，无需额外登录步骤。

Formail 的 `sso_cookie_name` 默认为 `_goauth_token`，与 GoAuth 默认值一致，无需修改。

---

## 角色说明

| GoAuth 角色 | Formail 角色 | 权限 |
|-------------|-------------|------|
| `admin` | `admin` | 可访问管理接口、查看所有表单数据 |
| `user` | `regular` | 仅可提交表单（不可访问后台） |

---

## 回退到独立模式

将 `config.toml` 改回：

```toml
[security]
auth_mode = "standalone"
```

重启后恢复独立 JWT 认证，原有账号数据不受影响。
