// Package version 提供 HomeBox 的唯一版本号来源。
//
// 版本号写在同目录的 version.ini 里：Go 侧用 go:embed 读取，
// 安装脚本（Inno Setup）用 ISPP 的 ReadIni 读同一个文件，
// 这样程序、接口、界面和安装包不会各写各的版本号。
package version

import (
	_ "embed"
	"strings"
)

//go:embed version.ini
var ini string

// Version 是 HomeBox 的版本号，例如 "1.1.0"。
var Version = parse(ini)

// parse 从 INI 文本里取出 version 值（文件里只有 [homebox] 一个节）。
func parse(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "version" {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return "0.0.0"
}
