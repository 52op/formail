package config

import (
	"errors"
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server       ServerConfig       `toml:"server"`
	Database     DatabaseConfig     `toml:"database"`
	Security     SecurityConfig     `toml:"security"`
	Spam         SpamConfig         `toml:"spam"`
	Admin        AdminConfig        `toml:"admin"`
	DirectAccess DirectAccessConfig `toml:"direct_access"`
	Log          LogConfig          `toml:"log"`
}

type DirectAccessConfig struct {
	Allow   bool   `toml:"allow"`
	KeyName string `toml:"key_name"`
	Key     string `toml:"key"`
}

type ServerConfig struct {
	Address     string `toml:"address"`
	AutoTLS     bool   `toml:"auto_tls"`
	ACMEmail    string `toml:"acme_email"`
	CertDataDir string `toml:"cert_data_dir"`
	HTTPSPort   string `toml:"https_port"`
	HTTPPort    string `toml:"http_port"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type SecurityConfig struct {
	JWTSecret     string `toml:"jwt_secret"`
	EncryptionKey string `toml:"encryption_key"`
}

type SpamConfig struct {
	RateLimitPerMinute int      `toml:"rate_limit_per_minute"`
	BlockedKeywords    []string `toml:"blocked_keywords"`
}

type AdminConfig struct {
	DefaultUsername string `toml:"default_username"`
	DefaultPassword string `toml:"default_password"`
}

type LogConfig struct {
	File string `toml:"file"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Address:     ":8080",
			AutoTLS:     false,
			ACMEmail:    "",
			CertDataDir: "./certmagic_data",
			HTTPSPort:   "443",
			HTTPPort:    "80",
		},
		Database: DatabaseConfig{Path: "./formail.db"},
		Security: SecurityConfig{
			JWTSecret:     "change-this-jwt-secret",
			EncryptionKey: "change-this-32-byte-encryption-key!!",
		},
		Spam: SpamConfig{
			RateLimitPerMinute: 30,
			BlockedKeywords:    []string{"viagra", "casino", "loan", "bitcoin", "porn"},
		},
		Admin: AdminConfig{
			DefaultUsername: "letvar@it0731.cn",
			DefaultPassword: "letvar",
		},
		DirectAccess: DirectAccessConfig{
			Allow:   true,
			KeyName: "x-access-key-name",
			Key:     "change_me",
		},
		Log: LogConfig{File: "./formail.log"},
	}
}

func Load(path string) (Config, error) {
	if path == "" {
		path = "config.toml"
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := Save(path, cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	content := fmt.Sprintf(`# Formail 配置文件

# 服务器配置
[server]
address = "%s"            # 非 AutoTLS 模式下的监听地址，如 :8080
auto_tls = %t             # 是否启用自动 HTTPS（CertMagic）
acme_email = "%s"         # ACME 邮箱，用于 Let's Encrypt 证书到期通知
cert_data_dir = "%s"      # 证书存储目录
https_port = "%s"         # HTTPS 端口（AutoTLS 模式）
http_port = "%s"          # HTTP 端口（ACME 验证 + 跳转）

# 数据库配置
[database]
path = "%s"               # SQLite 数据库文件路径

# 安全配置（请务必修改默认值！）
[security]
jwt_secret = "%s"         # JWT 签名密钥
encryption_key = "%s"     # AES-256 加密密钥（必须 32 字节）

# 反垃圾配置
[spam]
rate_limit_per_minute = %d  # 每个 IP 每分钟最大提交次数
blocked_keywords = [%s]     # 屏蔽关键词列表

# 管理员账号
[admin]
default_username = "%s"   # 默认管理员用户名
default_password = "%s"   # 默认管理员密码

# 直接访问控制（配合 Nginx 反向代理使用）
# allow = true  时：允许直接通过端口访问应用
# allow = false 时：必须通过反向代理，且请求头中携带正确的 key 才能访问
[direct_access]
allow = %t                # 是否允许直接访问
key_name = "%s"           # Nginx 需携带的请求头名称
key = "%s"                # Nginx 需携带的请求头值（请务必修改！）

# 日志配置
[log]
file = "%s"               # 日志文件路径
`,
		cfg.Server.Address, cfg.Server.AutoTLS, cfg.Server.ACMEmail,
		cfg.Server.CertDataDir, cfg.Server.HTTPSPort, cfg.Server.HTTPPort,
		cfg.Database.Path,
		cfg.Security.JWTSecret, cfg.Security.EncryptionKey,
		cfg.Spam.RateLimitPerMinute, formatKeywords(cfg.Spam.BlockedKeywords),
		cfg.Admin.DefaultUsername, cfg.Admin.DefaultPassword,
		cfg.DirectAccess.Allow, cfg.DirectAccess.KeyName, cfg.DirectAccess.Key,
		cfg.Log.File,
	)
	return os.WriteFile(path, []byte(content), 0644)
}

func formatKeywords(kw []string) string {
	result := ""
	for i, k := range kw {
		if i > 0 {
			result += ", "
		}
		result += `"` + k + `"`
	}
	return result
}
