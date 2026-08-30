package view

import (
	"html/template"
	"io"
	"io/fs"

	"github.com/labstack/echo/v5"

	"daohang/handler"
)

// SiteDataFunc 返回站点级数据（favicon 文件名、站点名），
// 由 main 包注入，供所有页面自动携带自定义图标与标题。
type SiteDataFunc func() (favicon, name string)

// Renderer 实现 echo.Renderer，基于标准库 html/template。
type Renderer struct {
	tmpl     *template.Template
	siteData SiteDataFunc
}

// New 从文件系统解析全部模板。
//
// 传入的 fsys 根目录必须是 templates 目录本身（见 main.go 中的 fs.Sub / os.DirFS），
// 因此这里的通配路径直接以 templates 为起点。
//
// 两点注意：
//  1. template.ParseFS 不支持 ** 递归通配，需逐层列出目录；
//  2. ParseFS 以文件名作为模板名，各目录下的模板文件名必须唯一。
func New(fsys fs.FS) (*Renderer, error) {
	tmpl, err := template.New("daohang").Funcs(funcMap()).ParseFS(fsys,
		"*.html",
		"admin/*.html",
	)
	if err != nil {
		return nil, err
	}
	return &Renderer{tmpl: tmpl}, nil
}

// SetSiteData 注入站点数据提供者，用于在渲染时自动补全 Favicon 与 SiteName。
func (r *Renderer) SetSiteData(fn SiteDataFunc) {
	r.siteData = fn
}

// Render 渲染指定模板。
//
// Echo v5 签名为 (c *Context, w io.Writer, name string, data any)，
// 与 v4 的 (w io.Writer, name string, data any, c Context) 参数顺序不同，切勿混用。
func (r *Renderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	// 若渲染数据是 map，注入后台路径前缀、CSRF token 与站点数据供模板使用。
	if m, ok := data.(map[string]any); ok {
		if _, exists := m["AdminBase"]; !exists {
			m["AdminBase"] = handler.AdminBase()
		}
		if ck, err := c.Cookie("daohang_csrf"); err == nil && ck.Value != "" {
			m["CSRFToken"] = ck.Value
		}
		// 统一补全站点数据（favicon、站点名），让所有页面都展示自定义图标与标题
		if r.siteData != nil {
			needFavicon := true
			if _, exists := m["Favicon"]; exists {
				needFavicon = false
			}
			needName := true
			if _, exists := m["SiteName"]; exists {
				needName = false
			}
			if needFavicon || needName {
				favicon, name := r.siteData()
				if needFavicon {
					m["Favicon"] = favicon
				}
				if needName {
					m["SiteName"] = name
				}
			}
		}
	}
	return r.tmpl.ExecuteTemplate(w, name, data)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		// seq 生成整数序列，用于模板中渲染列数选项
		"seq": func(from, to int) []int {
			out := make([]int, 0, to-from+1)
			for i := from; i <= to; i++ {
				out = append(out, i)
			}
			return out
		},
		// inc / dec 用于步进器边界判断
		"inc": func(i int) int { return i + 1 },
		"dec": func(i int) int { return i - 1 },
		// add / sub 用于分页页码计算
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}
}
