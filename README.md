# HomeBox

HomeBox 是一款运行在 Windows PC 上的**个人 / 家庭文件服务**：把电脑上的一个文件夹变成局域网内可访问的「家庭云盘」，手机、平板用浏览器即可浏览、上传、下载、搜索文件，还能像微信「文件传输助手」一样在手机和电脑之间互传文字、图片与文件。

- 🔒 管理员 + 访客两级账号，密码 bcrypt 加密
- 📱 手机浏览器访问，扫码即登录，无需安装 App
- 🖥️ 桌面托盘 + 本机管理面板（WebView2）
- 🚫 仅局域网可用，数据不出家门

---

## 功能

- **文件管理**：浏览、上传、下载、新建文件夹、重命名、复制、移动、删除。
- **搜索**：按文件名快速检索。
- **在线编辑**：在管理面板内直接编辑文本文件。
- **回收站**：删除的文件进入回收站，可一键清空 / 撤销。
- **隐藏文件夹**：管理员可把某些文件夹对访客隐藏，并在 Windows 中加上隐藏属性。
- **文件传输助手**：类似微信「文件传输助手」，PC 与手机之间互发文字、图片和文件；附件持久化保存，重启不丢。
- **对话管理**：在管理面板按日期范围查询聊天内容与附件，并可清空与某个对象的整个对话。
- **账号权限**：唯一管理员（admin）+ 多个访客（guest）。
- **局域网扫码登录**：管理面板显示局域网地址与二维码，手机扫码即在浏览器打开登录页。
- **独立存储空间**：可基于 VHDX 创建并挂载独立的虚拟磁盘作为存储空间。
- **深浅色主题**：跟随系统或手动切换。

---

## 快速开始

### 方式一：安装包（推荐）

1. 下载 [最新安装包](https://github.com/Tanger1ne-1/HomeBox/releases) `HomeBox-Setup-1.0.0.exe`，双击安装。
   - 安装过程会自动添加 Windows 防火墙入站规则（放行 TCP 8377）。
2. 启动 HomeBox（桌面快捷方式或开始菜单）。
3. 首次启动会弹出向导：创建管理员账号，并可指定要共享的文件夹（也可稍后再设）。
4. 完成后，托盘图标常驻右下角。

### 方式二：从源码运行

需要 Go 1.27+（Windows），见下方「构建」。

### 手机端登录

1. 让**手机与电脑连接同一个 WiFi / 局域网**。
2. 打开电脑上的 HomeBox 管理面板，进入 **⚙️ 设置 → 📶 局域网访问**。
3. 用手机**扫描二维码**，浏览器会自动打开 `http://<电脑IP>:8377/` 登录页。
   - 也可以直接点托盘菜单「打开手机访问链接」，或手动输入该地址。
4. 输入管理员账号（或访客账号）登录。

> 💡 电脑的局域网 IP 一般由路由器 DHCP 动态分配，**可能随时间变化**。每次扫码时管理面板都会读取当前 IP，所以以扫码为准即可；若想固定，可在路由器里为电脑做「IP / MAC 绑定（静态 DHCP 保留）」。

---

## 构建

需要 Go 1.27+（Windows）：

```bash
go build -o nasd.exe -ldflags "-H=windowsgui" ./cmd/nasd
```

`-H=windowsgui` 让程序不弹出控制台窗口；去掉该参数可在控制台看到日志。

`cmd/nasd/rsrc.syso` 内嵌了应用图标（Windows 资源），已随仓库提交，`go build` 即带出 HomeBox 图标。

### 更换图标

图标由 `icon.svg`（设计稿）+ `icon.ico`（多尺寸）组成，可用脚本重新生成圆角图标：

```bash
py scripts/make_icon.py          # 生成 icon.ico
go install github.com/akavel/rsrc@latest
rsrc -ico icon.ico -o cmd/nasd/rsrc.syso   # 重新内嵌图标
go build -o nasd.exe -ldflags "-H=windowsgui" ./cmd/nasd
```

### 打包 Windows 安装包

安装脚本 `installer/homebox.iss`（Inno Setup 6）：

```bash
cd installer
ISCC.exe homebox.iss             # 输出到 ../dist/HomeBox-Setup-1.0.0.exe
```

---

## 目录结构

```
pc/
├── cmd/nasd/            # 入口（含 rsrc.syso 内嵌图标）
├── internal/
│   ├── config/          # 配置读写（nasd.json）
│   ├── server/          # HTTP 服务与 API（移动端 + 管理面板）
│   ├── chat/            # 聊天 / 文件传输持久化存储
│   ├── files/           # 文件系统操作与路径校验
│   ├── thumbs/          # 图片缩略图
│   ├── desktop/         # 托盘图标 + WebView2 窗口
│   └── web/             # 静态资源嵌入（go:embed）
├── installer/           # Inno Setup 安装脚本
├── scripts/             # 图标生成等辅助脚本
├── icon.svg / icon.ico  # 应用图标
└── go.mod / go.sum
```

---

## 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-root <目录>` | 指定共享根目录（未设置则读取 / 生成 `nasd.json`） |
| `-port <端口>` | 移动端监听端口（默认 8377） |
| `-headless` | 仅启动服务，不弹窗口（调试用） |

## 默认端口

| 用途 | 端口 |
| --- | --- |
| 移动端 / 局域网访问 | `8377` |
| 本机管理面板（仅 127.0.0.1） | `8380` |

## 运行时数据

- `nasd.json` —— 配置与账号（含密码哈希，**切勿提交到版本库**）。
- `nasd.log` —— 日志（位于 exe 同目录）。
- 共享根目录下的 `.nas_chat/`、`.nas_trash/`、`.nas_thumb/` —— 聊天附件、回收站与缩略图。

---

## 许可证

[GPL-3.0](./LICENSE)
