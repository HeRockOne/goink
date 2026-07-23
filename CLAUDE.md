# CLAUDE.md

Goink — 桌面 AI 写作系统，Wails (Go + React) 构建。

## 构建

```bash
make deps      # 下载运行时依赖
make build     # 生产构建
make dev       # 开发模式
```
## ***重要***
1：当前运行环境：中国大陆**windows10**PowerShell 7**
2：除非必要，否则请严格并行读取文件或者代码
3：Windows 一键构建：`.\build.ps1` 或 `build.bat`

**目录规范：**
- Git 命令在项目根目录执行
- 前端命令在 `frontend/` 目录执行，用 `npm run build`
- Go 命令在项目根目录执行
- 系统依赖：`libsqlite3-dev libgtk-3-dev libwebkit2gtk-4.1-dev gcc`

## 项目结构

详见 [README.md](README.md#项目结构)

```
app/           # Wails 绑定 + HTTP API
internal/      # 核心逻辑（agent/llm/session/character/...）
mobile/        # 移动端 Web 前端
frontend/      # 桌面端 React 前端
docs/          # 文档
skills/        # 内置 Skill
```

## Key conventions

- **Build tags**: ONNX 和 sqlite-vec 代码使用 `//go:build cgo`
- **Data dir**: Linux/macOS `~/Goink/`，Windows exe 同级目录。含 `models/`、`runtime/`、`novel-agent.db`、`novels/`
- **Per-novel git repos**: 每本小说在 `{DataDir}/novels/{id}/`，含 `chapters/NNN.md`、`outlines/NNN.md`、`goink.md`
- **Character relationships**: 追加式，`is_current` 标记当前状态，旧记录保留为历史
- **Timeline entries**: `target_chapter` 仅用于 ORDER BY，不用于 WHERE（LLM 估算不精确）
- **Messages**: 追加式，版本化用于压缩
- **Commit style**: 英文，具体描述，无 Co-Authored-By，无 emoji
- **User communicates in Chinese** — 用中文回复

## CGO / ONNX

- ONNX embedder / VectorStore / RefreshQueue 均为全局单例
- `ResolveOnnxLib()` 搜索链：`<appdir>/runtime/` → `~/Goink/runtime/` → 系统路径
- Vec0 表：`vec_novel_{id}`，余弦距离
- Chunks: 420 tokens, 50 overlap
- Embedding: 512-dim, CLS pooling + L2 normalization

## No-gos

- 不要删除或修改日志/注释，除非明确要求
- 不要未经许可修改代码
- 不要用 sed/python 脚本改代码，用 Edit/Write 工具
- 不要主动问"commit?"或"开始写？"
- 不要手改 `frontend/src/lib/wailsjs/go/models.ts`（Wails 自动生成）
- Windows 上 cgo 编译诊断报错是预期行为

