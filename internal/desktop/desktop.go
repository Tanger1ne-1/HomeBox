// Package desktop 提供 HomeBox 的桌面端外壳：系统托盘 + 管理面板原生窗口。
// 管理面板本身是一个本地 Web 界面，这里用 WebView2 把它包成真正的桌面窗口。
package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"nas/internal/server"
)

// AdminPort 是管理面板的本地监听端口。
const AdminPort = 8380

// Run 启动桌面 UI（托盘为主循环，窗口按需打开）。阻塞直到用户从托盘退出。
func Run(app *server.App) {
	systray.Run(func() {
		setupTray(app)
		openWindow()
	}, func() {
		os.Exit(0)
	})
}

func setupTray(app *server.App) {
	systray.SetIcon(trayIcon())
	systray.SetTitle("HomeBox")
	systray.SetTooltip("HomeBox 文件服务")

	mOpen := systray.AddMenuItem("打开管理面板", "打开设置窗口")
	mLink := systray.AddMenuItem("打开手机访问链接", "用浏览器打开访问链接")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出 HomeBox 服务")

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openWindow()
			case <-mLink.ClickedCh:
				openLink(app)
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

// openWindow 在独立 goroutine 里打开管理面板窗口，可多次打开/关闭。
// WebView2 要求窗口的创建与消息循环必须在同一线程，且该线程需初始化 COM（STA），
// 否则窗口会因线程迁移而失去消息泵、表现为「卡死」。这里显式锁定线程并初始化 COM。
func openWindow() {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
		defer windows.CoUninitialize()

		w := webview.NewWithOptions(webview.WebViewOptions{
			Debug:     false,
			AutoFocus: true,
			WindowOptions: webview.WindowOptions{
				Title:  "HomeBox 管理面板",
				Width:  900,
				Height: 720,
				Center: true,
			},
		})
		if w == nil {
			return
		}
		defer w.Destroy()
		w.Navigate(adminURL())
		w.Run() // 窗口关闭即返回，不影响托盘与后台服务
	}()
}

// openLink 用默认浏览器打开手机访问链接（优先公网 IPv6，其次局域网）。
func openLink(app *server.App) {
	v4, v6 := server.PublicURLs(app.Config().Port)
	// 局域网优先：托盘「打开手机访问链接」默认打开局域网地址。
	url := v4
	if url == "" {
		url = v6
	}
	if url == "" {
		url = adminURL()
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func adminURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/", AdminPort)
}

// trayIcon 生成 32x32 的「房子 + 存储抽屉」托盘图标。
// Windows 的 systray 要求 .ico 格式，这里把 PNG 包装成单图 ICO（PNG-in-ICO）。
func trayIcon() []byte {
	const n = 32
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	accent := color.RGBA{47, 111, 224, 255} // #2f6fe0
	white := color.RGBA{255, 255, 255, 255}
	slot := color.RGBA{29, 78, 216, 255} // #1d4ed8

	fillRounded(img, n, 7, accent)
	fillTriangle(img, 16, 6, 5, 17, 27, 17, white) // 屋顶
	fillRect(img, 8, 16, 24, 27, white)            // 房身
	for _, y := range []int{19, 22, 25} {          // 存储抽屉（3 条横杠）
		fillRect(img, 11, y, 21, y+1, slot)
	}

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	return icoFromPNG(pngBuf.Bytes(), n)
}

// fillRect 填充 [x0,x1)×[y0,y1) 的矩形。
func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.Set(x, y, c)
		}
	}
}

// fillTriangle 用重心符号法填充一个三角形（包含边界）。
func fillTriangle(img *image.RGBA, ax, ay, bx, by, cx, cy int, c color.RGBA) {
	b := img.Bounds()
	for y := min3(ay, by, cy); y <= max3(ay, by, cy); y++ {
		for x := min3(ax, bx, cx); x <= max3(ax, bx, cx); x++ {
			if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
				continue
			}
			if inTriangle(x, y, ax, ay, bx, by, cx, cy) {
				img.Set(x, y, c)
			}
		}
	}
}

func inTriangle(px, py, ax, ay, bx, by, cx, cy int) bool {
	d1 := (px-bx)*(ay-by) - (ax-bx)*(py-by)
	d2 := (px-cx)*(by-cy) - (bx-cx)*(py-cy)
	d3 := (px-ax)*(cy-ay) - (cx-ax)*(py-ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func max3(a, b, c int) int {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

// fillRounded 填充一个 n×n 的圆角方块，圆角外为透明。
func fillRounded(img *image.RGBA, n, corner int, c color.RGBA) {
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			in := true
			if x < corner && y < corner && (x-corner)*(x-corner)+(y-corner)*(y-corner) > corner*corner {
				in = false
			}
			if x >= n-corner && y < corner && (x-(n-corner))*(x-(n-corner))+(y-corner)*(y-corner) > corner*corner {
				in = false
			}
			if x < corner && y >= n-corner && (x-corner)*(x-corner)+(y-(n-corner))*(y-(n-corner)) > corner*corner {
				in = false
			}
			if x >= n-corner && y >= n-corner && (x-(n-corner))*(x-(n-corner))+(y-(n-corner))*(y-(n-corner)) > corner*corner {
				in = false
			}
			if in {
				img.Set(x, y, c)
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
}

// icoFromPNG 把一张正方形 PNG 包装成单图 ICO 文件（Vista+ 支持 PNG-in-ICO）。
func icoFromPNG(pngBytes []byte, size int) []byte {
	var b bytes.Buffer
	// ICONDIR：保留 2 字节 = 0，类型 2 字节 = 1（图标），数量 2 字节 = 1
	b.Write([]byte{0, 0, 1, 0, 1, 0})
	// ICONDIRENTRY（16 字节）
	w := byte(size)
	if size >= 256 {
		w = 0 // 0 表示 256px
	}
	b.WriteByte(w)               // 宽
	b.WriteByte(w)               // 高
	b.WriteByte(0)               // 调色板颜色数
	b.WriteByte(0)               // 保留
	b.Write([]byte{1, 0, 32, 0}) // 平面数 1、位深 32（小端）
	n := uint32(len(pngBytes))
	b.Write([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}) // 图像数据大小
	b.Write([]byte{22, 0, 0, 0})                                         // 图像数据偏移 = 6 + 16
	b.Write(pngBytes)
	return b.Bytes()
}
