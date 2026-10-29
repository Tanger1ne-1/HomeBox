package config

import (
	"encoding/json"
	"os"
)

// 账号角色：唯一的管理员 admin，其余为访客 guest。
const (
	RoleAdmin = "admin"
	RoleGuest = "guest"
)

// Account 是一个可登录账号。
type Account struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"` // bcrypt 哈希
	Role         string `json:"role"`         // admin | guest
}

// Config 保存 HomeBox 服务的持久化配置。
type Config struct {
	RootDir  string    `json:"rootDir"`  // HomeBox 根目录（绝对路径）
	Port     int       `json:"port"`     // 监听端口
	External bool      `json:"external"` // 是否允许外网访问
	Accounts []Account `json:"accounts"` // 账号列表（含唯一管理员）

	// 设备名：对话功能里 PC 一方显示的名称，留空则用主机名。
	DeviceName string `json:"deviceName"`

	// 隐藏文件夹：管理员设为隐藏的文件夹对手机端访客不可见，且在 Windows 里加隐藏属性。
	HiddenFolders []string `json:"hiddenFolders"` // 隐藏的文件夹相对路径（URL 风格，以 / 开头）

	// 旧版字段：仅用于从旧配置迁移到 Accounts，之后不再使用。
	Username     string `json:"username,omitempty"`
	PasswordHash string `json:"passwordHash,omitempty"`
}

// Admin 返回唯一的管理员账号（无则 nil）。
func (c Config) Admin() *Account {
	for i := range c.Accounts {
		if c.Accounts[i].Role == RoleAdmin {
			return &c.Accounts[i]
		}
	}
	return nil
}

// Find 按用户名查找账号（无则 nil）。
func (c Config) Find(username string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].Username == username {
			return &c.Accounts[i]
		}
	}
	return nil
}

// HasAuth 表示是否已设置管理员账号（即是否完成首次配置）。
func (c Config) HasAuth() bool { return c.Admin() != nil }

// migrate 把旧版单账号字段迁移到 Accounts 列表。
func (c *Config) migrate() {
	if len(c.Accounts) == 0 && c.Username != "" && c.PasswordHash != "" {
		c.Accounts = []Account{{Username: c.Username, PasswordHash: c.PasswordHash, Role: RoleAdmin}}
		c.Username = ""
		c.PasswordHash = ""
	}
}

// Load 读取配置文件，不存在时返回零值 Config（不报错）。
func Load(path string) (*Config, error) {
	cfg := &Config{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.migrate()
	return cfg, nil
}

// Save 将配置写入文件。
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
