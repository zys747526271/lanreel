// Package webui 提供前端静态资源（单二进制部署，N-18）。
//
// 构建脚本会把 web/dist 的产物拷贝到 internal/webui/dist 后编译进二进制；
// 开发阶段可通过 app.web_dir 指向本地目录，实现前后端分离调试。
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// Assets 返回内嵌的前端资源。第二个返回值表示是否包含构建产物（index.html）。
func Assets() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return sub, false
	}
	return sub, true
}
