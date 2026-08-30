# Sr网址导航

单文件、零外部依赖的个人网址导航站。后端 Go + SQLite，前端模板与静态资源通过 `go:embed` 编进可执行文件，生产环境只需要一个二进制和它旁边的数据目录。

## 特性

- **分类 + 网址**：分类可排序，网址支持标题、描述、图标、排序与批量删除
- **三种图标**：Emoji（`🛠️`）、图片地址（`https://…/a.png`）、Font Awesome 类名（`fa-solid fa-house`）
- **三种展示模式**：大图标、图标列表、文字列表三种布局，每行显示 1–12 个网址或自适应铺满，默认主题可选浅色 / 暗色 / 跟随系统
- **显示开关**：网址链接文字、网址图标、分类图标可分别控制显示或隐藏
- **分类筛选**：前台按分类快速过滤
- **外观可调**：背景图上传与切换、站点名、浅色 / 暗色两套 Logo、自定义 favicon
- **响应式布局**：一套页面同时适配电脑和手机，640px / 380px 断点重排，卡片列数随屏幕宽度自动铺满，后台侧栏与表格在窄屏下同样可用
- **首页可见性**：可设置首页是否需要登录才能访问
- **后台管理**：分类与网址增删改、修改密码、登录日志、数据备份导出 / 导入
- **书签导入**：直接解析浏览器导出的 Netscape Bookmark HTML，按文件夹归类后勾选导入
- **可自定义后台路径**：默认 `/admin`，改完自动重启生效
- **纯 Go SQLite**：`modernc.org/sqlite`，`CGO_ENABLED=0` 静态编译，不依赖任何系统库

## 快速开始

### 编译

```bash
# 单个平台（当前系统）
go build -ldflags="-s -w" -o daohang .

# 四个平台一次产出（Windows PowerShell）
.\build.ps1
```

`build.ps1` 会在 `dist/` 下生成 windows / linux × amd64 / arm64 四个目录，每个目录包含可执行文件、`LICENSE`、`THIRD_PARTY_LICENSES.txt` 和空的 `data/`。

### 运行

```bash
# 设置会话密钥（必须，否则使用内置默认值，任何人都能伪造登录）
export SESSION_SECRET="$(head -c 32 /dev/urandom | base64)"

./daohang
```

启动后访问 `http://localhost:8080`，首次会自动跳转到 `/admin/setup` 创建管理员账号。初始化入口只在该账号创建前开放。

### 环境变量

全部可选，均有默认值：

| 变量 | 说明 | 默认值 |
|---|---|---|
| `ADDR` | 监听地址 | `:8080` |
| `DB_PATH` | SQLite 数据库文件路径 | `data/daohang.db` |
| `SESSION_SECRET` | 会话 Cookie 的 HMAC 签名密钥 | 内置默认值（不安全，务必覆盖） |
| `SECURE_COOKIE` | 置 `1` 时 Cookie 仅通过 HTTPS 下发 | `0` |
| `TRUST_PROXY` | 置 `1` 时信任 `X-Forwarded-For`，仅用于 Nginx 等反向代理之后 | `0` |
| `DEV` | 置 `1` 时从磁盘读取模板与静态资源，改动后刷新即生效 | 未设置 |

也可以复制 `.env.example` 为 `.env`，配合 `start.sh`（Linux）或 `start.ps1`（Windows）启动。

### systemd

仓库内提供 `daohang.service` 模板，改好部署路径与 `SESSION_SECRET` 后：

```bash
sudo cp daohang.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now daohang
```

### Docker

多阶段构建，运行层基于 Alpine，镜像约 35 MB：

```bash
docker build -t srdaohang:latest .

docker run -d --name srdaohang \
  -p 8080:8080 \
  -e SESSION_SECRET="$(head -c 32 /dev/urandom | base64)" \
  -v srdaohang-data:/app/data \
  --restart unless-stopped \
  srdaohang:latest
```

也可以直接用仓库内的 `docker-compose.yml`：

```bash
export SESSION_SECRET="$(head -c 32 /dev/urandom | base64)"
docker compose up -d
```

`SESSION_SECRET` 未设置时 compose 会直接报错退出，不会用内置默认密钥启动。

- 容器内以 uid `10001` 运行，数据库与上传图片都在卷 `/app/data`
- 想改用宿主目录，把 compose 里的 `srdaohang-data:/app/data` 换成 `./data:/app/data`，并先执行 `mkdir -p data && sudo chown -R 10001:10001 data`
- 镜像内 `/app/licenses/` 附带 `LICENSE` 与 `THIRD_PARTY_LICENSES.txt`
- 构建默认走国内模块代理，海外网络可加 `--build-arg GOPROXY=https://proxy.golang.org,direct`

## 数据与备份

所有数据都在 `DB_PATH` 指向的 SQLite 文件里，上传的图片在 `data/uploads/`。迁移或备份时把整个 `data/` 目录一起拷走即可；后台也提供导出 / 导入。

## 开发

```bash
DEV=1 go run .
```

`DEV=1` 时模板与 CSS/JS 直接从磁盘读取，改完刷新浏览器即可看到效果；`.go` 文件改动仍需重新运行。

## 目录结构

```
main.go              入口：配置加载、模板与静态资源嵌入、路由注册
config/              环境变量与默认值
handler/             请求处理：站点、后台、认证、导入、备份、设置
model/               数据结构
store/               SQLite 访问层与建表语句
view/                模板渲染器
  templates/         前台与后台页面（html/template）
static/              自有 CSS/JS 与 Font Awesome Free
build.ps1            四平台交叉编译与打包
```

## 许可

本项目以 [MIT License](LICENSE) 发布。

随二进制或源码分发的第三方组件、版权声明与完整许可条款见 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。其中 Font Awesome Free 的图标为 CC BY 4.0、字体为 SIL OFL 1.1、CSS 为 MIT；`modernc.org/sqlite` 等 Go 依赖为 BSD-3-Clause。
