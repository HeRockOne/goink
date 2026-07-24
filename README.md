<h1 align="center">Goink<br><sub>桌面 AI 写作系统 — Agent 实时决策 × 结构化记忆 × 写完自检</sub></h1>

<p align="center"><strong><a href="README_EN.md">English</a> | 中文</strong></p>

---

> **基于 [sigpanic/goink](https://github.com/sigpanic/goink) fork，在原版基础上新增移动端 Web 前端、HTTP API、Token 认证、阶段门禁、数据备份等功能。**
>
> **关于本项目：** 本 fork 使用 [MiMoCode](https://github.com/nicepkg/Aide) + [MiMo-V2.5](https://huggingface.co/XiaomiMiMo/MiMo-V2.5)（小米开源免费大模型）全程 AI 辅助开发。本人非程序员，未系统学习过任何代码，第一次 fork 练手，所有代码修改均由大模型完成，本人仅基于实际写作需求提出功能要求。无代码 review 能力，也无时间精力处理 bug 反馈。如有问题，请自行 fork 仓库修改。

---

## 目录

- [新增功能](#新增功能)
- [安装](#安装)
- [核心能力](#核心能力)
- [项目结构](#项目结构)
- [主题配色](#主题配色)
- [图标替换](#图标替换)
- [从源码构建](#从源码构建)
- [技术栈](#技术栈)
- [License](#license)

---

## 新增功能

### 移动端 Web 前端

手机浏览器访问 `https://{局域网IP}:8877/mobile/`，完整写作系统：

| 模块 | 功能 |
|------|------|
| 书架 | 小说列表、字数统计、当前书籍标识 |
| 小说详情 | 章节/角色/时间线/弧线/读者/偏好/地点 七大模块 |
| 全屏阅读器 | 字号行距背景调节、左右翻页、章节目录、进度记忆 |
| AI 对话 | 流式 SSE、思考过程、会话历史、模型切换 |
| 设置 | 深浅模式、中英语言、Token 管理、模型选择 |

> 首次连接支持扫码或手动输入 Token。离线时自动从 IndexedDB 读取缓存数据。

### HTTP API

完整 RESTful API，详见 [mobile/API.md](mobile/API.md)。

```
GET  /api/novels              小说列表
GET  /api/novels/{id}/chapters  章节列表
POST /api/chat                AI 对话（SSE）
GET  /api/settings/model      模型设置
...  共 17 个端点
```

### API Token 认证

所有 API（除健康检查）需要 Bearer Token：

```
Authorization: Bearer <token>
```

- 桌面端「设置 → API 认证令牌」查看/重置
- 移动端首次连接弹出输入框
- WebSocket 通过 `?token=<token>` 传递

### 阶段门禁

AI 写作流程强制阶段校验，详见 [docs/phase-gate.md](docs/phase-gate.md)：

```
prepare → outline → write → review → maintain → prepare
```

- 每阶段有工具白名单和完成条件
- 可在设置中开关
- API 对话同样生效

### 其他新增

| 功能 | 说明 |
|------|------|
| 字数配置 | 自定义章节最少/最多字数 |
| WebDAV | 内置服务器，手机文件管理器阅读小说 |
| 模型管理 | model.dev 自动获取、思考模式支持 |
| 数据备份 | 一键备份/恢复全量数据（含数据库、小说、用户技能） |
| 会话管理 | 删除历史会话 |
| 日志开关 | 设置中启用/禁用文件日志 |
| 对话优化 | 消息气泡、Markdown 渲染、复制按钮 |
| 复古牛皮纸主题 | 浅色/深色双模式，泛黄旧书风格 + Diff 编辑器主题 |
| 双端实时同步 | 桌面端与移动端 WebSocket 全双工同步对话 |
| 移动端扫码连接 | 桌面端显示 Token 二维码，手机扫码快速连接 |
| 移动端离线存储 | IndexedDB 缓存，离线可阅读小说和查看设定 |
| 自动 HTTPS | 启动时自动生成证书，手机摄像头可用 |
| 模型服务商显示 | 模型选择显示 `provider / model-name` 格式 |
| 端口冲突处理 | 启动时自动杀掉占用端口的旧进程 |
| 技能 Token 优化 | 17 个 Skill 精简 84%，工具描述加 token 提示 |
| 子代理事件同步 | 子代理（review/memory）事件同步到移动端 |
| config.json 移除 | 数据目录直接用 exe 位置，无需 config.json |

---

## 项目结构

```
goink/
├── app/                    # Wails 绑定层
│   ├── api_server.go       #   HTTP API + Token 认证
│   ├── chat.go             #   对话入口
│   ├── settings.go         #   设置 + API Token
│   ├── backup.go           #   数据备份/恢复
│   └── ...                 #   其他模块
├── internal/
│   ├── agent/              # LLM 对话循环、压缩、子 Agent
│   ├── mcp_tools/          # 31 MCP 工具（含 token 优化提示）
│   ├── llm/                # 多提供商 LLM 传输
│   ├── session/            # 会话 + 消息
│   ├── character/          # 角色 + 有向关系图
│   ├── timeline/           # 伏笔 + 章节计划
│   ├── storyarc/           # 故事弧线
│   ├── reader/             # 读者视角
│   ├── location/           # 地点图
│   ├── rag/                # 向量搜索（ONNX）
│   ├── ws/                 # WebSocket 同步（wspulse）
│   ├── cert/               # 自动 HTTPS 证书生成
│   ├── webdav/             # WebDAV 服务器
│   └── ...
├── mobile/                 # 移动端 Web 前端（纯 JS + wspulse）
│   ├── index.html          #   入口
│   ├── app.js              #   主逻辑 + 离线存储
│   ├── style.css           #   样式
│   ├── jsQR.js             #   QR 码扫描库
│   ├── wspulse.mjs         #   WebSocket 客户端
│   └── API.md              #   API 文档
├── frontend/               # 桌面端（React + TypeScript）
├── docs/                   # 文档
├── skills/                 # 内置写作 Skill（已优化 token）
├── build.ps1               # 一键构建部署
└── build.bat               # 一键构建部署
```

---

## 核心能力

### Agent 自主决策

31 个结构化工具，LLM 自主决定调用哪个、传什么参数。不是 pipeline——Agent 在当前对话中查角色、查伏笔、读写正文、更新状态，直到完成。

写完后自动注入维护检查：角色变化、伏笔回收、弧线推进、读者认知。可启动审稿子 Agent 独立审计。

### 本地语义搜索

BGE 中文模型 ONNX 本地推理 + sqlite-vec 向量索引。问"那个吊坠"能找到没写"吊坠"但暗示它的段落。无需网络。

### 结构化创作状态

| 模块 | 能力 |
|------|------|
| 角色 | 有向关系图，变化保留历史 |
| 伏笔 | 目标回收章节 + 重要度 + 异常提醒 |
| 弧线 | 节点链，写完自动推进 |
| 地点 | 层级包含 + 空间连通图 |
| 读者 | 已知/悬念/误解追踪 |
| 偏好 | 全局 + 单书两层管理 |

### Skill 系统

三层覆盖（小说 > 用户 > 内置）× 三种模式（auto/manual/always）= 9 种策略。`.md` 文件即新 Skill，零代码扩展。

### 风格蒸馏

贴一段样文，AI 从六个维度拆解生成仿写 Skill。`/风格名` 一键加载。

### Diff 审批

每次编辑生成 Diff 预览，批准后才写入。所有修改有 Git 历史，随时回退。

---

## 安装

从 [Releases](https://github.com/HeRockOne/goink/releases) 下载安装包。

### 运行时依赖

| 依赖 | 说明 |
|------|------|
| WebView2 Runtime | Windows 11 内置；Windows 10 需要安装，程序首次启动会自动下载 |
| LLM API Key | 兼容 OpenAI 格式（DeepSeek、OpenAI、Claude 等均可） |

程序自带 Git 和 ONNX Runtime（向量搜索），无需额外安装。

### 从源码构建

#### 编译环境要求

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | 1.21+ | 后端编译 |
| Node.js | 18+ | 前端构建 |
| MSYS2 | - | Windows 必须，提供 gcc + 头文件 |
| Wails CLI | v2.13+ | 框架构建工具 |

#### Windows 编译环境安装（必须）

项目使用 CGO（SQLite、sqlite-vec 向量搜索），Windows 上必须通过 MSYS2 提供 gcc 和完整的 POSIX 头文件。**不要使用 TDM-GCC**，会缺少 `sqlite3.h` 等头文件导致编译失败。

```powershell
# 1. 安装 MSYS2（https://www.msys2.org），默认路径 C:\msys64

# 2. 打开 MSYS2 终端，安装工具链
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-pkgconf

# 3. 将 MSYS2 加入系统 PATH
# C:\msys64\mingw64\bin
```

#### 构建步骤

```bash
# 克隆仓库
git clone https://github.com/HeRockOne/goink.git
cd goink

# Linux 依赖（Ubuntu/Debian）
sudo apt install libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc

# 构建
make deps
make build   # 生产构建
make dev     # 开发模式
```

#### Windows 一键构建

```powershell
.\build.ps1    # PowerShell
build.bat      # CMD
```

## 主题配色

复古牛皮纸风格，浅色/深色双模式。颜色定义在 `frontend/src/index.css`。

### 颜色变量 → 文件位置

| 变量 | 作用 | 所在文件 |
|------|------|----------|
| `--background` | 整体页面背景 | `index.css` |
| `--foreground` | 全局文字颜色 | `index.css` |
| `--card` | 卡片/面板背景 | `index.css` |
| `--primary` | 主色调（按钮、链接） | `index.css` |
| `--sidebar` | 侧边栏背景 | `index.css` |
| `--border` | 边框颜色 | `index.css` |
| `--muted` | 次要背景（输入框、代码块） | `index.css` |
| `--reader-bg` | 阅读区域外层背景 | `index.css` + `ContentPanel.css` |
| `--reader-paper` | 阅读正文纸张背景 | `index.css` + `ContentPanel.css` |
| `--bubble-user` | 用户消息气泡 | `index.css` |

### 各区域对应关系

| 区域 | 浅色模式 | 深色模式 |
|------|----------|----------|
| 页面整体背景 | `--background` `#f5edd6` | `--background` `#1a1210` |
| 侧边栏 | `--sidebar` `#ede5ce` | `--sidebar` `#1e1612` |
| 阅读正文区 | `--reader-paper` `#faf4e4` | `--reader-paper` `#2a1e16` |
| 卡片/弹窗 | `--card` `#faf4e4` | `--card` `#2a1e16` |
| Diff 编辑器 | Monaco 自定义主题 `goink-light` | Monaco 自定义主题 `goink-dark` |

### Monaco 编辑器主题

Diff 视图使用自定义 Monaco 主题（`ContentPanel.tsx` 中注册）：
- `goink-light`：浅色纸张背景 `#f5edd6`，插入行绿底，删除行红底
- `goink-dark`：深棕皮革背景 `#2a1e16`，暖白文字

### 修改配色步骤

1. 编辑 `frontend/src/index.css` 中 `:root`（浅色）或 `[data-theme="dark"]`（深色）
2. 阅读区域额外受 `frontend/src/components/content/ContentPanel.css` 控制
3. Monaco Diff 编辑器主题在 `ContentPanel.tsx` 中注册
4. 运行 `npm run build` 验证无报错
5. 运行 `.\build.ps1` 重新构建部署

---

## 图标替换

项目有多个图标位置，替换后重新构建即可生效：

| 位置 | 用途 | 格式 |
|------|------|------|
| `build/windows/icon.ico` | exe 文件图标 + 窗口标题栏图标 | ICO（多尺寸） |
| `appicon.png` | Wails 构建用的应用图标 | PNG |
| `frontend/public/logo.svg` | 标题栏左上角 Logo | SVG |
| `frontend/public/favicon.svg` | 浏览器标签页图标 | SVG |
| `assets/logo.svg` | Logo 源文件 | SVG |

### 替换步骤

1. 准备新图标（推荐 SVG 或高清 PNG）
2. 替换对应文件：
   - **exe 图标**：用在线工具（如 convertio.co/png-ico）将 PNG 转为 ICO，替换 `build/windows/icon.ico`
   - **应用图标**：将 PNG 放到项目根目录，重命名为 `appicon.png`，同时复制到 `build/appicon.png`
   - **标题栏 Logo**：将 SVG 放到 `frontend/public/logo.svg`
   - **Favicon**：将 SVG 放到 `frontend/public/favicon.svg`
3. 运行 `.\build.ps1` 重新构建
4. 若 exe 图标未更新，清除 Windows 图标缓存或重启电脑

---

## 技术栈

| 层 | 选型 |
|---|---|
| Agent 引擎 | ReAct 循环（Go，SSE + 31 工具 + 子 Agent） |
| 桌面框架 | Wails v2（Go + WebView） |
| 前端 | React 19 + TypeScript + Tailwind 4 + shadcn/ui |
| 移动端 | HTTP API + 纯原生 JS Web 前端 |
| 数据库 | SQLite + GORM |
| 向量搜索 | sqlite-vec + ONNX Runtime |
| 版本控制 | 内置 Git |
| 安全 | 双层沙箱 + 审批流 + API Token 认证 |
| 局域网 | WebDAV 服务器 |

---

## License

AGPL-3.0。详见 [LICENSE](LICENSE) 和 [NOTICE](NOTICE)。
