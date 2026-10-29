package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var staticFS embed.FS

// Handler 返回服务前端静态资源的处理器。
// 前端资源标记为 no-store，避免开发期更新被浏览器缓存，刷新即生效。
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // 仅在打包缺失时发生
	}
	fsHandler := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fsHandler.ServeHTTP(w, r)
	})
}

// AdminHandler 返回 PC 端管理面板页面（admin.html），仅在本机端口上提供。
func AdminHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/icon.svg":
			data, err := staticFS.ReadFile("static/icon.svg")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		case "/", "/index.html", "/admin.html":
			data, err := staticFS.ReadFile("static/admin.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	})
}
