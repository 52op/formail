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
- 邮件渠道管理：
  - 内置：QQ、163、Outlook/Hotmail（预置 SMTP 参数）
  - 自定义：SMTP / IMAP（IMAP用于连通性测试）
  - 启用/禁用、优先级、测试连通性
- 邮件容错：按优先级渠道自动切换发送
- 提交记录：查询、删除、CSV 导出
- 安全与治理：
  - JWT 后台鉴权
  - 提交数据加密存储（AES-GCM）
  - CORS 全域支持
  - IP 限流
  - 蜜罐字段拦截
  - 关键词过滤

---

## 2. 技术栈

- Go 1.21+
- Gin
- SQLite（`modernc.org/sqlite`，纯 Go 驱动）
- 前端：原生 HTML + CSS + JavaScript

---

## 3. 项目目录结构

```/dev/null/tree.txt#L1-29
Formail/
├─ cmd/formail/main.go                # 应用入口、路由与中间件挂载
├─ internal/
│  ├─ config/config.go                # 配置读取与默认配置
│  ├─ db/
│  │  ├─ db.go                        # SQLite 连接与 schema 初始化
│  │  ├─ models.go                    # 核心模型结构体
│  │  └─ seed.go                      # 默认管理员初始化
│  ├─ handlers/
│  │  ├─ context.go                   # Handler 上下文
│  │  ├─ auth.go                      # 登录、改密
│  │  ├─ forms.go                     # 表单管理 API
│  │  ├─ channels.go                  # 邮件渠道管理 API + 测试
│  │  └─ submissions.go               # 提交接收、记录查询、导出
│  ├─ middleware/
│  │  ├─ cors.go                      # 跨域
│  │  ├─ ratelimit.go                 # IP 限流
│  │  └─ auth.go                      # JWT 鉴权
│  ├─ services/
│  │  ├─ mailer.go                    # 邮件发送与失败切换
│  │  ├─ spam.go                      # 垃圾检测
│  │  ├─ template.go                  # 模板渲染
│  │  └─ imap.go                      # IMAP 连通性测试
│  └─ utils/                          # 响应/JWT/加密/密码/token 工具
├─ web/static/
│  ├─ css/style.css                   # 后台样式
│  ├─ js/api.js                       # API 请求封装
│  ├─ js/layout.js                    # 后台布局
│  └─ pages/*.html                    # 登录/表单/渠道/提交/账号页面
├─ examples/static-form-example.html  # 静态网站接入示例
├─ config.json                        # 运行配置
├─ go.mod / go.sum                    # 依赖定义
└─ README.md                          # 文档
```

---

## 4. 配置说明（重点修改）

编辑 `config.json`：

- `server.address`：监听地址，默认 `:8080`
- `database.path`：SQLite 数据库路径
- `security.jwt_secret`：JWT 密钥（**必须改**）
- `security.encryption_key`：数据加密密钥（**必须改**）
- `spam.rate_limit_per_minute`：IP 每分钟提交上限
- `spam.blocked_keywords`：垃圾关键词
- `admin.default_username` / `admin.default_password`：首次启动时默认管理员

> 注意：默认管理员仅在 `users` 表为空时初始化一次。

---

## 5. 启动与部署

### 5.1 本地运行

1) 安装 Go 1.21+

2) 进入项目目录：

```/dev/null/cmd.sh#L1-2
cd Formail
go mod tidy
```

3) 启动：

```/dev/null/cmd.sh#L1-1
go run ./cmd/formail
```

看到输出：`Formail running on :8080` 即表示成功。

访问：

- 登录页：`http://127.0.0.1:8080/`
- 表单管理：`http://127.0.0.1:8080/admin/forms`

### 5.2 生产构建

```/dev/null/cmd.sh#L1-2
go build -o formail ./cmd/formail
./formail
```

Windows 可执行文件为 `formail.exe`。

### 5.3 一体化部署说明

该项目前端静态页由后端统一托管（`/assets` + `/admin/*`），不依赖 Node 或额外前端服务。

---

## 6. 默认管理员账号

- 用户名：`admin`
- 密码：`admin123`

首次登录后建议进入「账号设置」立即修改。

---

## 7. 后端 API 一览

### 公开接口

- `POST /api/auth/login` 密码登录
- `POST /api/auth/login-code` 邮箱验证码登录
- `GET /api/auth/captcha` 获取图形验证码（SVG）
- `POST /api/auth/send-code` 发送邮箱验证码（register/login，含图形验证码校验）
- `POST /api/auth/register` 邮箱验证码注册（受开放注册开关控制）
- `GET /api/auth/approve?token=...` 审核通过（一次性）
- `GET /api/auth/reject?token=...` 审核拒绝（一次性）
- `POST /f/:token` 静态表单提交

### 需鉴权（Authorization: Bearer <token>）

- 用户（当前登录）
  - `GET /api/auth/me`
  - `POST /api/auth/change-password`
- 用户管理（仅管理员)
  - `GET /api/users`
  - `POST /api/users`
  - `PUT /api/users/:id`
  - `DELETE /api/users/:id`
  - `POST /api/users/:id/approve`（后台审核通过）
  - `POST /api/users/:id/reject`（后台审核拒绝）
  - `POST /api/users/:id/reset-password`
- 系统设置（仅管理员)
  - `GET /api/settings/registration`
  - `PUT /api/settings/registration`
  - 可配置：开放注册、注册默认状态、管理员通知邮箱、验证码邮件主题/正文模板、邮件签名、验证码冷却秒数、图形验证码开关、图形验证码有效期、设备指纹限流阈值、服务邮件渠道ID（为空则自动选优先级最高启用渠道）
- 表单
  - `GET /api/forms`
  - `GET /api/forms/:id`
  - `POST /api/forms`
  - `PUT /api/forms/:id`
  - `DELETE /api/forms/:id`
- 渠道
  - `GET /api/channels`
  - `POST /api/channels`
  - `PUT /api/channels/:id`
  - `DELETE /api/channels/:id`
  - `POST /api/channels/:id/test`
- 提交记录
  - `GET /api/submissions?form_id=1`
  - `DELETE /api/submissions/:id`
  - `GET /api/submissions/export?form_id=1`

---

## 8. 静态网站接入示例

### 8.1 HTML 直接提交

```/dev/null/form-example.html#L1-10
<form action="https://your-domain.com/f/FORM_TOKEN" method="post">
  <input name="name" required>
  <input type="email" name="email" required>
  <textarea name="message" required></textarea>
  <input name="_gotcha" style="display:none" tabindex="-1" autocomplete="off">
  <button type="submit">发送</button>
</form>
```

### 8.2 JSON fetch 提交

```/dev/null/form-fetch.js#L1-12
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
- IMAP 通道用于“连通性测试”，不用于邮件发送。

---

## 10. 开放注册开关说明

- 管理员可在后台「用户管理」页面配置“允许公开注册普通用户”
- 默认关闭（`allow_register=0`）
- 开启后，访客可通过登录页下方注册入口调用 `POST /api/auth/register`
- 注册用户默认角色为 `user`，默认状态为启用

## 11. 无第三方服务依赖说明

- 数据库使用本地 SQLite 文件
- 前端为纯静态资源
- 后端单进程可运行
- 唯一外部依赖是你实际发信所用的邮件服务商（QQ/163/Outlook/企业 SMTP）

---

## 12. 常见问题

1) 登录失败：确认 `users` 表是否已初始化，以及密码是否被修改。
2) 提交后不发信：检查渠道是否启用、优先级、授权码是否正确、SMTP 是否可连通。
3) 导出无内容：确认筛选条件是否正确，或是否存在提交记录。
4) 改了默认管理员但未生效：已有用户时不会再次自动覆盖初始化。

---

## 13. 后续可扩展方向（建议）

- 多用户与 RBAC 权限
- Webhook 转发（Slack/飞书/企业微信）
- 附件上传与对象存储
- 更完整的模板变量系统（if/loop）
- 防重放签名与 CAPTCHA
- 审计日志与告警通知
