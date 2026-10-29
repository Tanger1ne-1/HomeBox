package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	"nas/internal/config"
	"nas/internal/files"
	"nas/internal/web"
)

// NewAdmin 返回 PC 端管理面板的 HTTP 处理器（应只绑定 127.0.0.1）。
// 管理面板需要管理员登录，除登录/首次向导相关接口外均要求管理员会话。
func NewAdmin(app *App) http.Handler {
	mux := http.NewServeMux()

	// 登录与状态
	mux.HandleFunc("POST /api/admin/login", app.handleAdminLogin)
	mux.HandleFunc("POST /api/admin/logout", app.handleAdminLogout)
	mux.HandleFunc("GET /api/admin/state", app.handleAdminState)
	mux.HandleFunc("POST /api/admin/setup", app.handleAdminSetup)
	mux.HandleFunc("GET /api/admin/status", app.handleAdminStatus)
	mux.HandleFunc("POST /api/admin/accounts", app.handleAdminAccounts)

	// 设置
	mux.HandleFunc("POST /api/config", app.handleAdminConfigSet)
	mux.HandleFunc("GET /api/disks", handleDisks)
	mux.HandleFunc("POST /api/vhdx", app.handleVHDX)
	mux.HandleFunc("GET /api/qr", handleQR)
	mux.HandleFunc("POST /api/shutdown", handleShutdown)

	// 文件浏览/管理（复用手机端接口，管理员专用）
	mux.HandleFunc("GET /api/list", app.handleList)
	mux.HandleFunc("GET /api/file", app.handleFile)
	mux.HandleFunc("GET /api/thumb", app.handleThumb)
	mux.HandleFunc("POST /api/upload", app.handleUpload)
	mux.HandleFunc("POST /api/delete", app.handleDelete)
	mux.HandleFunc("POST /api/move", app.handleMove)
	mux.HandleFunc("POST /api/create", app.handleCreate)
	mux.HandleFunc("POST /api/rename", app.handleRename)
	mux.HandleFunc("POST /api/copy", app.handleCopy)
	mux.HandleFunc("GET /api/search", app.handleSearch)
	mux.HandleFunc("POST /api/save", app.handleSave)
	mux.HandleFunc("POST /api/undo", app.handleUndo)
	mux.HandleFunc("POST /api/trash/empty", app.handleEmptyTrash)
	mux.HandleFunc("GET /api/chat/state", app.handleChatState)
	mux.HandleFunc("GET /api/chat/messages", app.handleChatMessages)
	mux.HandleFunc("POST /api/chat/text", app.handleChatText)
	mux.HandleFunc("POST /api/chat/upload", app.handleChatUpload)
	mux.HandleFunc("GET /api/chat/query", app.handleChatQuery)
	mux.HandleFunc("POST /api/chat/clear", app.handleChatClear)

	// 隐藏文件夹
	mux.HandleFunc("POST /api/hidden", app.handleHidden)

	mux.Handle("/", web.AdminHandler())

	return app.adminAuthMiddleware(mux)
}

// adminAuthMiddleware 要求管理员会话，放行登录页、首次向导与状态查询。
func (a *App) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch p {
		case "/", "/index.html", "/admin.html", "/api/admin/login", "/api/admin/state", "/api/admin/setup":
			next.ServeHTTP(w, r)
			return
		}
		if !a.validAdminSession(tokenFromRequest(r)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleAdminState 供登录页判断当前应显示登录、首次向导还是主界面。
func (a *App) handleAdminState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"authed":   a.validAdminSession(tokenFromRequest(r)),
		"firstRun": !a.RequiresAuth(),
	})
}

// handleAdminLogin 管理员登录。
func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	token, err := a.AdminLogin(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	writeJSON(w, map[string]any{"ok": true})
}

// handleAdminLogout 管理员退出登录。
func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	a.Logout(tokenFromRequest(r))
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

// handleAdminSetup 首次使用：创建管理员账号并自动登录（仅在尚无管理员时可用）。
func (a *App) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	u := strings.TrimSpace(req.Username)
	if u == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "请输入用户名"})
		return
	}
	if len(req.Password) < 4 {
		writeJSON(w, map[string]any{"ok": false, "error": "密码至少 4 位"})
		return
	}
	if a.RequiresAuth() {
		writeJSON(w, map[string]any{"ok": false, "error": "已存在管理员账号"})
		return
	}
	var setErr error
	err := a.Update(func(c *config.Config) {
		if c.Admin() != nil {
			setErr = errors.New("已存在管理员账号")
			return
		}
		h, e := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if e != nil {
			setErr = e
			return
		}
		c.Accounts = append(c.Accounts, config.Account{Username: u, PasswordHash: string(h), Role: config.RoleAdmin})
	})
	if err == nil {
		err = setErr
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 创建成功后自动登录（下发管理员会话），便于继续配置存储。
	token, lerr := a.AdminLogin(u, req.Password)
	if lerr != nil {
		writeJSON(w, map[string]any{"ok": false, "error": lerr.Error()})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
	writeJSON(w, map[string]any{"ok": true})
}

// handleAdminStatus 返回主界面所需状态：账号列表、移动端在线、存储/外网配置。
func (a *App) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	c := a.Config()
	info := collectExtInfo(c.Port)
	writeJSON(w, map[string]any{
		"me":           a.currentUsername(tokenFromRequest(r)),
		"mobileOnline": a.MobileOnline(),
		"accounts":     a.accountStatuses(),
		"rootDir":      c.RootDir,
		"deviceName":   a.deviceName(),
		"port":         c.Port,
		"external":     c.External,
		"hasAuth":      c.HasAuth(),
		"ipv4":         info.IPv4,
		"ipv6":         info.IPv6,
		"url":          info.URL,
		"urlV4":        info.URLV4,
	})
}

// handleAdminAccounts 账号管理：新增访客 / 删除访客 / 修改密码。
func (a *App) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action   string `json:"action"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var actErr error
	var revokeUser string
	err := a.Update(func(c *config.Config) {
		switch req.Action {
		case "addGuest":
			u := strings.TrimSpace(req.Username)
			if u == "" {
				actErr = errors.New("请输入用户名")
				return
			}
			if len(req.Password) < 4 {
				actErr = errors.New("密码至少 4 位")
				return
			}
			if c.Find(u) != nil {
				actErr = errors.New("该用户名已存在")
				return
			}
			h, e := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if e != nil {
				actErr = e
				return
			}
			c.Accounts = append(c.Accounts, config.Account{Username: u, PasswordHash: string(h), Role: config.RoleGuest})

		case "delete":
			u := strings.TrimSpace(req.Username)
			acct := c.Find(u)
			if acct == nil {
				actErr = errors.New("账号不存在")
				return
			}
			if acct.Role == config.RoleAdmin {
				actErr = errors.New("不能删除管理员账号")
				return
			}
			for i := range c.Accounts {
				if c.Accounts[i].Username == u {
					c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
					break
				}
			}
			revokeUser = u

		case "setPassword":
			u := strings.TrimSpace(req.Username)
			if len(req.Password) < 4 {
				actErr = errors.New("密码至少 4 位")
				return
			}
			for i := range c.Accounts {
				if c.Accounts[i].Username == u {
					h, e := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
					if e != nil {
						actErr = e
						return
					}
					c.Accounts[i].PasswordHash = string(h)
					revokeUser = u
					return
				}
			}
			actErr = errors.New("账号不存在")

		default:
			actErr = errors.New("未知操作")
		}
	})
	if err == nil {
		err = actErr
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if revokeUser != "" {
		a.revokeSessions(revokeUser)
	}
	writeJSON(w, map[string]any{"ok": true, "accounts": a.accountStatuses()})
}

// handleAdminConfigSet 更新存储位置 / 外网开关 / 设备名，立即生效并写回配置。
func (a *App) handleAdminConfigSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RootDir    string  `json:"rootDir"`
		External   *bool   `json:"external"`
		DeviceName *string `json:"deviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	var setErr error
	err := a.Update(func(c *config.Config) {
		if req.RootDir != "" {
			abs, e := filepath.Abs(req.RootDir)
			if e != nil {
				setErr = e
				return
			}
			if st, e := os.Stat(abs); e != nil || !st.IsDir() {
				setErr = fmt.Errorf("目录不存在或不是文件夹：%s", req.RootDir)
				return
			}
			c.RootDir = abs
		}
		if req.External != nil {
			c.External = *req.External
		}
		if req.DeviceName != nil {
			c.DeviceName = strings.TrimSpace(*req.DeviceName)
		}
	})
	if err == nil {
		err = setErr
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleShutdown 退出整个程序（先回包，稍后退出）。
func handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
}

// extInfo 汇总本机对外访问信息（局域网 IPv4 / 公网 IPv6 / 访问链接）。
type extInfo struct {
	IPv4  string `json:"ipv4"`
	IPv6  string `json:"ipv6"`
	URL   string `json:"url"`
	URLV4 string `json:"urlV4"`
}

func collectExtInfo(port int) extInfo {
	var info extInfo
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return info
	}
	for _, ad := range addrs {
		ipn, ok := ad.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP
		if ip.To4() != nil {
			if info.IPv4 == "" && ip.To4().IsPrivate() {
				info.IPv4 = ip.String()
			}
		} else if info.IPv6 == "" && isGlobal(ip) {
			info.IPv6 = ip.String()
		}
	}
	if info.IPv6 != "" {
		info.URL = fmt.Sprintf("http://[%s]:%d/", info.IPv6, port)
	}
	if info.IPv4 != "" {
		info.URLV4 = fmt.Sprintf("http://%s:%d/", info.IPv4, port)
	}
	return info
}

// PublicURLs 返回 (局域网访问链接, 公网访问链接)，供桌面托盘等使用。
func PublicURLs(port int) (string, string) {
	info := collectExtInfo(port)
	return info.URLV4, info.URL
}

// handleQR 把 ?text= 编码成二维码 PNG。
func handleQR(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		http.Error(w, "missing text", http.StatusBadRequest)
		return
	}
	png, err := qrcode.Encode(text, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

// handleDisks 列出本机磁盘分区与剩余空间（供管理面板选择存储位置）。
func handleDisks(w http.ResponseWriter, r *http.Request) {
	out, err := runPS(`
$ErrorActionPreference = 'Stop'
$disks = @(Get-CimInstance Win32_LogicalDisk | Select-Object DeviceID,VolumeName,FileSystem,@{n='Size';e={[int64]$_.Size}},@{n='Free';e={[int64]$_.FreeSpace}},DriveType)
$disks | ConvertTo-Json -Compress | Out-File -FilePath $ResultFile -Encoding utf8
`)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var disks []struct {
		DeviceID   string `json:"DeviceID"`
		VolumeName string `json:"VolumeName"`
		FileSystem string `json:"FileSystem"`
		Size       int64  `json:"Size"`
		Free       int64  `json:"Free"`
		DriveType  int    `json:"DriveType"`
	}
	if err := json.Unmarshal([]byte(out), &disks); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "解析磁盘信息失败：" + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "disks": disks})
}

// handleVHDX 创建并挂载一个 VHDX 虚拟磁盘作为独立存储空间（需管理员权限）。
func (a *App) handleVHDX(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentDir string `json:"parentDir"` // .vhdx 文件存放目录
		SizeGB    int    `json:"sizeGB"`    // 容量（GB）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.SizeGB <= 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "容量必须大于 0 GB"})
		return
	}
	if st, err := os.Stat(req.ParentDir); err != nil || !st.IsDir() {
		writeJSON(w, map[string]any{"ok": false, "error": "存放位置不存在或不是文件夹"})
		return
	}
	sizeBytes := int64(req.SizeGB) * 1024 * 1024 * 1024

	res, err := os.CreateTemp("", "nas-vhdx-result-*.txt")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	resPath := res.Name()
	res.Close()
	defer os.Remove(resPath)

	script := fmt.Sprintf(vhdxScript, psQ(req.ParentDir), sizeBytes, psQ(resPath))
	out, err := runElevatedPS(script, resPath)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "创建失败：" + err.Error()})
		return
	}
	switch {
	case strings.HasPrefix(out, "OK:"):
		writeJSON(w, map[string]any{"ok": true, "letter": strings.TrimPrefix(out, "OK:")})
	case strings.HasPrefix(out, "ERR:"):
		writeJSON(w, map[string]any{"ok": false, "error": strings.TrimPrefix(out, "ERR:")})
	default:
		writeJSON(w, map[string]any{"ok": false, "error": "未知结果：" + out})
	}
}

// psQ 把字符串转成 PowerShell 单引号字面量（转义内部单引号）。
func psQ(s string) string { return strings.ReplaceAll(s, "'", "''") }

// vhdxScript 是创建 VHDX 独立空间的 PowerShell 脚本模板。
const vhdxScript = `$ErrorActionPreference = 'Stop'
$parentDir = '%s'
$sizeBytes = %d
$resultFile = '%s'
try {
  $vhdx = Join-Path $parentDir 'HomeBox.vhdx'
  if (Test-Path $vhdx) { throw "VHDX 文件已存在: $vhdx" }
  New-VHD -Path $vhdx -SizeBytes $sizeBytes -Dynamic | Out-Null
  Mount-VHD -Path $vhdx | Out-Null
  $disk = Get-Disk | Where-Object { $_.Location -and $_.Location -eq $vhdx } | Select-Object -First 1
  if (-not $disk) { throw "挂载后未找到磁盘" }
  Initialize-Disk -Number $disk.Number -PartitionStyle GPT
  $part = New-Partition -DiskNumber $disk.Number -UseMaximumSize -AssignDriveLetter
  $letter = $part.DriveLetter
  Format-Volume -DriveLetter $letter -FileSystem NTFS -NewFileSystemLabel 'HomeBox' -Confirm:$false | Out-Null
  "OK:$letter" | Out-File -FilePath $resultFile -Encoding utf8
} catch {
  "ERR:$($_.Exception.Message)" | Out-File -FilePath $resultFile -Encoding utf8
}
`

// runPS 执行一段脚本（非管理员）。脚本内可用 $ResultFile 写入结果，返回其内容（去 BOM）。
func runPS(script string) (string, error) {
	res, err := os.CreateTemp("", "nas-ps-result-*.txt")
	if err != nil {
		return "", err
	}
	resPath := res.Name()
	res.Close()
	defer os.Remove(resPath)

	ps, err := os.CreateTemp("", "nas-ps-*.ps1")
	if err != nil {
		return "", err
	}
	defer os.Remove(ps.Name())
	if _, err := ps.WriteString("param([string]$ResultFile)\n" + script); err != nil {
		ps.Close()
		return "", err
	}
	ps.Close()

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", ps.Name(), "-ResultFile", resPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errBuf.String()))
	}
	data, err := os.ReadFile(resPath)
	if err != nil {
		return "", err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return strings.TrimSpace(string(data)), nil
}

// runElevatedPS 以管理员权限（弹 UAC）执行脚本，脚本把结果写入 resultFile。
func runElevatedPS(script, resultFile string) (string, error) {
	ps, err := os.CreateTemp("", "nas-vhdx-*.ps1")
	if err != nil {
		return "", err
	}
	defer os.Remove(ps.Name())
	if _, err := ps.WriteString(script); err != nil {
		ps.Close()
		return "", err
	}
	ps.Close()

	outer := fmt.Sprintf("Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile -ExecutionPolicy Bypass -File %s'", ps.Name())
	cmd := exec.Command("powershell", "-NoProfile", "-Command", outer)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var errBuf bytes.Buffer
	cmd.Stdout = &errBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(errBuf.String()))
	}
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return "", fmt.Errorf("读取结果失败：%v", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return strings.TrimSpace(string(data)), nil
}

// handleHidden 管理员设置/取消一个文件夹为隐藏（手机端访客不可见，Windows 加隐藏属性）。
func (a *App) handleHidden(w http.ResponseWriter, r *http.Request) {
	if !a.isAdminAccount(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Path   string `json:"path"`
		Hidden bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	full, err := files.Resolve(a.RootDir(), req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "路径无效"})
		return
	}
	root := filepath.Clean(a.RootDir())
	clean := filepath.Clean(full)
	if clean == root {
		writeJSON(w, map[string]any{"ok": false, "error": "不能隐藏根目录"})
		return
	}
	if filepath.Base(clean) == files.TrashName {
		writeJSON(w, map[string]any{"ok": false, "error": "不能隐藏回收站"})
		return
	}
	st, err := os.Stat(clean)
	if err != nil || !st.IsDir() {
		writeJSON(w, map[string]any{"ok": false, "error": "目标不存在或不是文件夹"})
		return
	}
	attr := "-h"
	if req.Hidden {
		attr = "+h"
	}
	attribCmd := exec.Command("attrib", attr, clean)
	attribCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 避免隐藏/取消隐藏时弹出控制台窗口
	if out, err := attribCmd.CombinedOutput(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "设置 Windows 隐藏属性失败：" + strings.TrimSpace(string(out))})
		return
	}
	rel, err := files.URLPath(req.Path)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "路径无效"})
		return
	}
	if err := a.ToggleHidden(rel, req.Hidden); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
