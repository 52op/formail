# Formail

Formail 是一个基于 Go 的轻量化静态网站表单接收与邮件转发系统，定位对标 Formspree，支持私有化部署、可视化后台管理、多邮件渠道、垃圾提交防护与提交数据管理。

## 1. 功能总览

- 静态表单无后端接入：直接向 `POST /f/{token}` 提交
- 支持提交格式：
  - `application/x-www-form-urlencoded`
  - `multipart/form-data`
  - `application/json`
- 多用户支持：管理员可管理普通用户账号、角色、状态、资料、密码重置
- 表单管理：新增/编辑/删除/查看、生成唯一提交地址
- 管理员可管理所有用户的表单（按用户筛选、编辑、删除）
- 邮箱验证：创建/修改接收邮箱时自动发送验证邮件，验证后表单才生效
- 邮件渠道管理：
  - 内置：QQ、163、Outlook/Hotmail（预置 SMTP 参数）
  - 自定义：SMTP / IMAP（IMAP用于连通性测试）
  - 启用/禁用、优先级、测试连通性
  - 管理员可共享渠道给普通用户，设置共享限制
- 邮件容错：按优先级渠道自动切换发送
- 提交记录：分页查询、删除、批量删除、CSV 导出
- 表单字段自定义：通过 `fields_schema` 定义字段名称、类型、是否必填，提交时自动校验
- Webhook 通知：表单提交后可推送到外部 URL，支持 HMAC-SHA256 签名
- 数据统计仪表盘：总表单/提交数、每日趋势图、表单排行、渠道统计
- 邮件队列可视化：查看待处理/失败邮件任务，手动重试
- 自动回复：表单提交后自动向提交者发送确认邮件
- 成功页面主题：蓝色/绿色/紫色/橙色/深色 5 种主题可选
- 安全与治理：
  - JWT 后台鉴权
  - 提交数据加密存储（AES-GCM）
  - CORS 全域支持
  - IP 限流（指纹级）
  - 蜜罐字段拦截
  - 关键词过滤
  - 图形验证码
  - 来源域名白名单
  - 反向代理访问控制（禁止直连，必须通过 Nginx 携带密钥头访问）
- CertMagic 自动 SSL：基于 Let's Encrypt 自动申请/续期证书，零配置 HTTPS
- TOML 配置文件：支持注释，便于理解各参数功能

---

## 2. 技术栈

- Go 1.25+
- Gin
- SQLite（`modernc.org/sqlite`，纯 Go 驱动）
- CertMagic（自动 TLS 证书管理）
- 前端：原生 HTML + CSS + JavaScript
- 图表：Chart.js（数据统计页面）

---

## 3. 项目目录结构

```
Formail/
├─ cmd/formail/main.go                # 应用入口、路由与中间件挂载
├─ internal/
│  ├─ config/config.go                # 配置读取与默认配置（TOML）
│  ├─ db/
│  │  ├─ db.go                        # SQLite 连接与 schema 初始化
│  │  ├─ models.go                    # 核心模型结构体
│  │  └─ seed.go                      # 默认管理员初始化
│  ├─ handlers/
│  │  ├─ context.go                   # Handler 上下文
│  │  ├─ auth.go                      # 登录、改密、注册、验证码
│  │  ├─ forms.go                     # 表单管理 API（含邮箱验证）
│  │  ├─ channels.go                  # 邮件渠道管理 API + 测试
│  │  ├─ submissions.go               # 提交接收、分页查询、批量删除、导出、Webhook
│  │  ├─ stats.go                     # 数据统计 API
│  │  ├─ queue.go                     # 邮件队列管理 API
│  │  ├─ users.go                     # 用户管理 API
│  │  └─ settings.go                  # 系统设置 API
│  ├─ middleware/
│  │  ├─ cors.go                      # 跨域
│  │  ├─ ratelimit.go                 # IP 限流
│  │  ├─ auth.go                      # JWT 鉴权
│  │  ├─ role.go                      # 角色校验
│  │  └─ reverse_proxy.go            # 反向代理访问控制
│  ├─ services/
│  │  ├─ mailer.go                    # 邮件发送与失败切换
│  │  ├─ spam.go                      # 垃圾检测
│  │  ├─ template.go                  # 模板渲染
│  │  └─ imap.go                      # IMAP 连通性测试
│  └─ utils/                          # 响应/JWT/加密/密码/token 工具
├─ web/static/
│  ├─ css/style.css                   # 后台样式
│  ├─ css/marketing.css               # 营销页样式
│  ├─ js/api.js                       # API 请求封装
│  ├─ js/layout.js                    # 后台布局
│  └─ pages/*.html                    # 登录/表单/渠道/提交/统计/队列/文档等页面
├─ examples/static-form-example.html  # 静态网站接入示例
├─ config.toml                        # 运行配置（首次启动自动生成）
├─ go.mod / go.sum                    # 依赖定义
└─ README.md                          # 文档
```

---

## 4. 配置说明（重点修改）

编辑 `config.toml`（首次启动自动生成带注释的默认配置）：

### 服务器配置 `[server]`
- `address`：非 AutoTLS 模式下的监听地址，默认 `:8080`
- `auto_tls`：是否启用 CertMagic 自动 HTTPS
- `acme_email`：ACME 邮箱（Let's Encrypt 证书到期通知）
- `cert_data_dir`：证书存储目录
- `https_port` / `http_port`：AutoTLS 模式下的端口

### 数据库 `[database]`
- `path`：SQLite 数据库路径

### 安全配置 `[security]`（**必须修改**）
- `jwt_secret`：JWT 密钥
- `encryption_key`：数据加密密钥（必须 32 字节）

### 反垃圾 `[spam]`
- `rate_limit_per_minute`：IP 每分钟提交上限
- `blocked_keywords`：垃圾关键词

### 管理员 `[admin]`
- `default_username` / `default_password`：首次启动时默认管理员

### 反向代理访问控制 `[direct_access]`
- `allow`：是否允许直连（`false` 则必须通过 Nginx 反向代理）
- `key_name`：Nginx 需携带的请求头名称
- `key`：请求头值（**必须修改**）

> 注意：默认管理员仅在 `users` 表为空时初始化一次。

---

## 5. 启动与部署

### 5.1 本地运行

1) 安装 Go 1.25+

2) 进入项目目录：

```bash
cd Formail
go mod tidy
```

3) 启动：

```bash
go run ./cmd/formail
```

看到输出：`Formail running on :8080` 即表示成功。

访问：

- 首页：`http://127.0.0.1:8080/`
- 登录页：`http://127.0.0.1:8080/login`
- 控制台：`http://127.0.0.1:8080/dashboard`

### 5.2 生产构建

```bash
go build -o formail ./cmd/formail
./formail
```

Windows 可执行文件为 `formail.exe`。

### 5.3 自动 HTTPS 部署

在 `config.toml` 中启用：

```toml
[server]
auto_tls = true
acme_email = "admin@your-domain.com"
```

启动后自动监听 443（HTTPS）和 80（HTTP 跳转），首次访问某域名时自动签发 Let's Encrypt 证书，后台自动续期。

### 5.4 Nginx 反向代理

如果启用 `direct_access.allow = false`，Nginx 需携带密钥头：

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header x-access-key-name change_me;  # 与配置文件中 key 一致
    }
}
```

### 5.5 一体化部署说明

该项目前端静态页由后端统一托管（`/assets` + `/dashboard/*`），不依赖 Node 或额外前端服务。

### 5.6 CLI 命令行工具

Formail 支持通过命令行参数执行管理操作。CLI 模式仅连接数据库执行更新，执行完毕立即退出，不会启动 Web 服务。可在服务运行时从另一个终端安全执行。

```bash
# 修改管理员用户名
./formail -admin newadmin@example.com

# 修改管理员密码
./formail -adminpwd newpassword123

# 同时修改
./formail -admin newadmin@example.com -adminpwd newpassword123
```

---

## 6. 默认管理员账号

- 用户名：`letvar@it0731.cn`
- 密码：`letvar`

首次登录后建议进入「个人资料」立即修改。

---

## 7. 后端 API 一览

### 公开接口

- `POST /api/auth/login` 密码登录
- `POST /api/auth/login-code` 邮箱验证码登录
- `GET /api/auth/captcha` 获取图形验证码（SVG）
- `POST /api/auth/verify-captcha` 校验图形验证码
- `POST /api/auth/send-code` 发送邮箱验证码（register/login）
- `POST /api/auth/register` 邮箱验证码注册（受开放注册开关控制）
- `GET /api/auth/approve?token=...` 审核通过（一次性）
- `GET /api/auth/reject?token=...` 审核拒绝（一次性）
- `GET /api/site-settings` 获取站点设置（公开）
- `POST /f/:token` 静态表单提交
- `GET /verify/:token` 邮箱验证

### 需鉴权（Authorization: Bearer \<token\>）

**用户**
- `GET /api/auth/me` 获取当前用户信息
- `POST /api/auth/change-password` 修改密码

**表单管理**
- `GET /api/forms` 表单列表（管理员可传 `user_id` 筛选）
- `GET /api/forms/:id` 表单详情
- `POST /api/forms` 创建表单
- `PUT /api/forms/:id` 更新表单
- `DELETE /api/forms/:id` 删除表单
- `POST /api/forms/:id/resend-verify` 重发验证邮件

**渠道管理**
- `GET /api/channels` 渠道列表
- `GET /api/channels/bindable` 可绑定渠道列表
- `POST /api/channels` 创建渠道
- `PUT /api/channels/:id` 更新渠道
- `DELETE /api/channels/:id` 删除渠道
- `POST /api/channels/:id/test` 测试渠道连通性

**提交记录**
- `GET /api/submissions?form_id=1&page=1&limit=20` 分页查询
- `DELETE /api/submissions/:id` 删除记录
- `POST /api/submissions/batch-delete` 批量删除（`{"ids":[1,2,3]}`）
- `GET /api/submissions/export?form_id=1` CSV 导出

**数据统计**
- `GET /api/stats/overview` 总览（总表单/总提交/今日/发送率）
- `GET /api/stats/daily?days=30` 每日趋势
- `GET /api/stats/forms` 表单排行
- `GET /api/stats/channels` 渠道统计

### 仅管理员

**用户管理**
- `GET /api/users` 用户列表
- `POST /api/users` 创建用户
- `PUT /api/users/:id` 更新用户
- `DELETE /api/users/:id` 删除用户
- `POST /api/users/:id/approve` 审核通过
- `POST /api/users/:id/reject` 审核拒绝
- `POST /api/users/:id/reset-password` 重置密码

**系统设置**
- `GET /api/settings/registration` 注册设置
- `PUT /api/settings/registration` 更新注册设置
- `PUT /api/site-settings` 更新站点设置
- `POST /api/site-settings/upload` 上传站点资源

**邮件队列**
- `GET /api/mail-queue?status=pending&page=1&limit=20` 队列列表
- `POST /api/mail-queue/:id/retry` 重试失败任务
- `DELETE /api/mail-queue/:id` 删除任务

---

## 8. 静态网站接入示例

### 8.1 HTML 直接提交

```html
<form action="https://your-domain.com/f/FORM_TOKEN" method="post">
  <input name="name" required>
  <input type="email" name="email" required>
  <textarea name="message" required></textarea>
  <input name="_gotcha" style="display:none" tabindex="-1" autocomplete="off">
  <button type="submit">发送</button>
</form>
```

### 8.2 JSON fetch 提交

```javascript
await fetch('https://your-domain.com/f/FORM_TOKEN', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    name: 'Alice',
    email: 'alice@example.com',
    message: 'hello',
    _gotcha: ''
  })
})
```

---

## 9. 邮件渠道配置建议

- 内置渠道（QQ/163/Outlook）只需填写账号 + 授权码，Host/Port 可留空走预置。
- 自定义 SMTP：填写 Host、Port、TLS、用户名、密码。
- `priority` 越小越优先，发送失败会自动切换到下一个启用渠道。
- IMAP 通道用于"连通性测试"，不用于邮件发送。
- 管理员可开启渠道共享，让普通用户绑定管理员的渠道。

---

## 10. 开放注册开关说明

- 管理员可在后台「系统设置」页面配置"允许公开注册普通用户"
- 默认关闭（`allow_register=0`）
- 开启后，访客可通过登录页下方注册入口调用 `POST /api/auth/register`
- 注册用户默认角色为 `user`，默认状态可配置
- 当默认状态设为"待审核"时，注册表单会出现必填的"注册申请"字段，用户需说明注册原因。该内容会随审核邮件发送给管理员，兼防机器人注册

---

## 11. 无第三方服务依赖说明

- 数据库使用本地 SQLite 文件
- 前端为纯静态资源
- 后端单进程可运行
- 唯一外部依赖是你实际发信所用的邮件服务商（QQ/163/Outlook/企业 SMTP）
- CertMagic 自动 HTTPS 需要 Let's Encrypt 外部服务（免费）

---

## 12. 常见问题

1) 登录失败：确认 `users` 表是否已初始化，以及密码是否被修改。
2) 提交后不发信：检查渠道是否启用、优先级、授权码是否正确、SMTP 是否可连通。可在「邮件队列」页面查看失败原因。
3) 导出无内容：确认筛选条件是否正确，或是否存在提交记录。
4) 忘记管理员密码：使用 CLI 工具重置：`./formail -adminpwd newpassword`
5) AutoTLS 证书签发失败：确认 80/443 端口是否开放，域名 DNS 是否指向服务器 IP。
6) 直连被拒绝：检查 `direct_access` 配置，确认是否需要通过 Nginx 反向代理访问。
