package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"nas/internal/config"
	"nas/internal/desktop"
	"nas/internal/server"
)

// setupLog 把日志写到 exe 同目录下的 nasd.log（GUI 程序没有控制台，日志落文件便于排查）。
func setupLog() {
	f, err := os.OpenFile("nasd.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
}

func main() {
	var (
		rootFlag = flag.String("root", "", "HomeBox 根目录（未设置则读取/生成 nasd.json）")
		portFlag = flag.Int("port", 0, "监听端口（默认 8377）")
		headless = flag.Bool("headless", false, "仅启动服务，不弹窗口（调试用）")
	)
	flag.Parse()
	setupLog()

	cfg, err := config.Load("nasd.json")
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if *rootFlag != "" {
		cfg.RootDir = *rootFlag
	}
	if *portFlag != 0 {
		cfg.Port = *portFlag
	}

	// 首次运行未配置根目录时不要直接退出，先启动管理面板，让首次向导引导用户
	// 设置存储位置与管理员账号（否则双击安装后的程序会因缺少 nasd.json 一闪而过）。
	if cfg.RootDir != "" {
		abs, err := filepath.Abs(cfg.RootDir)
		if err != nil {
			log.Fatalf("解析根目录失败: %v", err)
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			log.Fatalf("根目录不存在或不是目录: %s", abs)
		}
		cfg.RootDir = abs
	}
	if cfg.Port == 0 {
		cfg.Port = 8377
	}

	if err := config.Save("nasd.json", cfg); err != nil {
		log.Printf("保存配置失败（不影响运行）: %v", err)
	}

	app := server.NewApp(cfg, "nasd.json")

	// 手机端服务（局域网 + 可选外网）
	addr := fmt.Sprintf(":%d", cfg.Port)
	go func() {
		if err := http.ListenAndServe(addr, server.New(app)); err != nil {
			log.Fatalf("HomeBox 服务退出: %v", err)
		}
	}()

	// 管理面板（仅本机）
	adminAddr := fmt.Sprintf("127.0.0.1:%d", desktop.AdminPort)
	go func() {
		if err := http.ListenAndServe(adminAddr, server.NewAdmin(app)); err != nil {
			log.Fatalf("管理面板退出: %v", err)
		}
	}()

	log.Printf("HomeBox 服务启动")
	log.Printf("  根目录  : %s", cfg.RootDir)
	log.Printf("  访问地址: http://localhost:%d", cfg.Port)
	log.Printf("  管理面板: http://localhost:%d", desktop.AdminPort)
	if cfg.HasAuth() {
		log.Printf("  账号密码: 已启用")
	}
	if cfg.External {
		log.Printf("  外网访问: 已开启")
	}

	if *headless {
		select {} // 无窗口模式，保持运行
	} else {
		desktop.Run(app) // 桌面窗口 + 托盘（阻塞）
	}
}
