package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nas/internal/chat"
	"nas/internal/config"
	"nas/internal/files"
)

// sessionCookie 是登录后写入浏览器的会话 Cookie 名。
const sessionCookie = "nas_session"

// sessionActiveWindow 是「在线」判定窗口：会话在此时间内有过请求即视为在线。
const sessionActiveWindow = 5 * time.Minute

// 会话类型：手机端会话和管理面板会话彼此独立（同一 Cookie 名、不同主机）。
const (
	sessionMobile = "mobile"
	sessionAdmin  = "admin"
)

// Session 是一次登录会话。
type Session struct {
	Username string    `json:"username"`
	Role     string    `json:"role"`
	Kind     string    `json:"kind"` // mobile | admin
	Expires  time.Time `json:"-"`
	LastSeen time.Time `json:"-"`
}

// App 持有两个 HTTP 服务（手机端 + 管理面板）共享的运行状态：
// 当前配置与登录会话。配置可被管理面板在线修改，两个服务实时生效。
type App struct {
	mu   sync.RWMutex
	cfg  *config.Config
	path string // nasd.json 路径

	sessMu   sync.Mutex
	sessions map[string]*Session

	histMu  sync.Mutex
	history []UndoEntry

	chatMu    sync.Mutex
	chatStore *chat.Store
	chatRoot  string
}

// NewApp 创建共享状态。cfg 由 main 加载，path 用于回写配置。
func NewApp(cfg *config.Config, path string) *App {
	return &App{cfg: cfg, path: path, sessions: map[string]*Session{}}
}

// Config 返回配置快照（值拷贝，避免数据竞争）。
func (a *App) Config() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return *a.cfg
}

// RootDir 返回当前共享根目录。
func (a *App) RootDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.RootDir
}

// chat 返回当前共享根目录对应的对话存储（根目录变化时重建）。
func (a *App) chat() *chat.Store {
	root := a.RootDir()
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.chatStore == nil || a.chatRoot != root {
		a.chatStore = chat.Open(filepath.Join(files.ChatDir(root), "messages.json"))
		a.chatRoot = root
	}
	return a.chatStore
}

// Update 在锁内修改配置并写回 nasd.json。
func (a *App) Update(fn func(*config.Config)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	fn(a.cfg)
	return config.Save(a.path, a.cfg)
}

// RequiresAuth 表示当前是否需要登录。
func (a *App) RequiresAuth() bool { return a.Config().HasAuth() }

// ExternalEnabled 表示外网访问开关是否打开。
func (a *App) ExternalEnabled() bool { return a.Config().External }

// loginAs 校验账号密码并登记一个 30 天有效的会话。
// role 非空时额外要求账号角色匹配（管理面板登录用）。
func (a *App) loginAs(username, password, kind, role string) (string, error) {
	cfg := a.Config()
	acct := cfg.Find(username)
	if acct == nil {
		return "", errors.New("用户名或密码错误")
	}
	if role != "" && acct.Role != role {
		return "", errors.New("用户名或密码错误")
	}
	if bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte(password)) != nil {
		return "", errors.New("用户名或密码错误")
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	a.sessMu.Lock()
	a.sessions[token] = &Session{
		Username: username,
		Role:     acct.Role,
		Kind:     kind,
		Expires:  now.Add(30 * 24 * time.Hour),
		LastSeen: now,
	}
	a.sessMu.Unlock()
	return token, nil
}

// Login 手机端登录：管理员与访客均可，权限一致。
func (a *App) Login(username, password string) (string, error) {
	return a.loginAs(username, password, sessionMobile, "")
}

// AdminLogin 管理面板登录：仅管理员账号。
func (a *App) AdminLogin(username, password string) (string, error) {
	return a.loginAs(username, password, sessionAdmin, config.RoleAdmin)
}

// Logout 注销指定会话。
func (a *App) Logout(token string) {
	if token == "" {
		return
	}
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	delete(a.sessions, token)
}

// getSession 取会话（校验有效期并刷新 LastSeen），不存在/过期返回 nil。
func (a *App) getSession(token string) *Session {
	if token == "" {
		return nil
	}
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	s, ok := a.sessions[token]
	if !ok {
		return nil
	}
	now := time.Now()
	if now.After(s.Expires) {
		delete(a.sessions, token)
		return nil
	}
	s.LastSeen = now
	cp := *s
	return &cp
}

// validMobileSession 手机端鉴权：仅接受手机端会话。
func (a *App) validMobileSession(token string) bool {
	s := a.getSession(token)
	return s != nil && s.Kind == sessionMobile
}

// validAdminSession 管理面板鉴权：仅接受管理员的管理会话。
func (a *App) validAdminSession(token string) bool {
	s := a.getSession(token)
	return s != nil && s.Kind == sessionAdmin && s.Role == config.RoleAdmin
}

// currentUsername 返回会话对应的用户名（无则空串）。
func (a *App) currentUsername(token string) string {
	if s := a.getSession(token); s != nil {
		return s.Username
	}
	return ""
}

// revokeSessions 注销某账号的所有会话（删除账号 / 改密码后踢下线）。
func (a *App) revokeSessions(username string) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	for tok, s := range a.sessions {
		if s.Username == username {
			delete(a.sessions, tok)
		}
	}
}

// AccountStatus 是账号管理页里的一项：账号 + 手机端在线状态。
type AccountStatus struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Online   bool   `json:"online"`
	Devices  int    `json:"devices"`  // 在线设备数
	LastSeen int64  `json:"lastSeen"` // Unix 秒，0 表示从未在线
}

// accountStatuses 汇总各账号的手机端在线情况。
func (a *App) accountStatuses() []AccountStatus {
	cfg := a.Config()
	now := time.Now()

	a.sessMu.Lock()
	type agg struct {
		devices  int
		lastSeen time.Time
	}
	online := map[string]agg{}
	for _, s := range a.sessions {
		if s.Kind != sessionMobile || now.After(s.Expires) || now.Sub(s.LastSeen) > sessionActiveWindow {
			continue
		}
		m := online[s.Username]
		m.devices++
		if s.LastSeen.After(m.lastSeen) {
			m.lastSeen = s.LastSeen
		}
		online[s.Username] = m
	}
	a.sessMu.Unlock()

	out := make([]AccountStatus, 0, len(cfg.Accounts))
	for _, acct := range cfg.Accounts {
		st := AccountStatus{Username: acct.Username, Role: acct.Role}
		if m, ok := online[acct.Username]; ok {
			st.Online = true
			st.Devices = m.devices
			st.LastSeen = m.lastSeen.Unix()
		}
		out = append(out, st)
	}
	return out
}

// MobileOnline 表示当前是否有手机设备在线（右上角指示灯用）。
func (a *App) MobileOnline() bool {
	for _, s := range a.accountStatuses() {
		if s.Online {
			return true
		}
	}
	return false
}

// tokenFromRequest 依次从 Cookie / Authorization / ?token= 取会话令牌。
func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		return strings.TrimPrefix(auth, prefix)
	}
	return r.URL.Query().Get("token")
}

// randomToken 生成随机会话令牌（十六进制）。
func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// isGlobal 判断来源 IP 是否为公网地址（外网开关关闭时据此拒绝公网访问）。
func isGlobal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return false
	}
	return ip.IsGlobalUnicast()
}

// remoteIP 从请求里解析客户端 IP。
func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// undoStep 是撤销里的一步：把 From（当前所在位置）rename 回 To（原位置）。
type undoStep struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// UndoEntry 是一次可撤销的文件操作（删除/移动）。
type UndoEntry struct {
	Kind  string     `json:"kind"` // delete | move
	Desc  string     `json:"desc"`
	Steps []undoStep `json:"steps"`
}

// pushUndo 记录一次可撤销操作（最多保留 20 条）。
func (a *App) pushUndo(e UndoEntry) {
	a.histMu.Lock()
	defer a.histMu.Unlock()
	a.history = append(a.history, e)
	if len(a.history) > 20 {
		a.history = a.history[len(a.history)-20:]
	}
}

// popUndo 取出最近一次可撤销操作（无则返回 nil）。
func (a *App) popUndo() *UndoEntry {
	a.histMu.Lock()
	defer a.histMu.Unlock()
	if len(a.history) == 0 {
		return nil
	}
	last := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	return &last
}

// removeUndoKind 移除指定类型的撤销条目（清空回收站时，删除类撤销随之失效）。
func (a *App) removeUndoKind(kind string) {
	a.histMu.Lock()
	defer a.histMu.Unlock()
	kept := a.history[:0]
	for _, e := range a.history {
		if e.Kind != kind {
			kept = append(kept, e)
		}
	}
	a.history = kept
}

// isAdminAccount 判断请求是否来自管理员账号（管理面板或手机端登录的管理员均可）。
func (a *App) isAdminAccount(r *http.Request) bool {
	s := a.getSession(tokenFromRequest(r))
	return s != nil && s.Role == config.RoleAdmin
}

// HiddenFolders 返回已隐藏文件夹的 URL 风格相对路径列表。
func (a *App) HiddenFolders() []string { return a.Config().HiddenFolders }

// IsHiddenPath 判断相对路径 p 是否等于某个隐藏文件夹或在其内部。
func (a *App) IsHiddenPath(p string) bool {
	p = "/" + strings.Trim(p, "/")
	if p == "/" {
		return false
	}
	for _, h := range a.Config().HiddenFolders {
		if h == p || strings.HasPrefix(p, h+"/") {
			return true
		}
	}
	return false
}

// ToggleHidden 在配置里加入或移除一个隐藏文件夹路径（不触碰文件系统）。
func (a *App) ToggleHidden(p string, hidden bool) error {
	return a.Update(func(c *config.Config) {
		list := c.HiddenFolders
		if hidden {
			for _, h := range list {
				if h == p {
					return
				}
			}
			c.HiddenFolders = append(list, p)
		} else {
			kept := list[:0]
			for _, h := range list {
				if h != p {
					kept = append(kept, h)
				}
			}
			c.HiddenFolders = kept
		}
	})
}
