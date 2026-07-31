# 内置 Provider 配置状态（联网核实 2026-08-01）

> 内置 provider 模板（`internal/llm/providers.go`）是**默认参考**。用户实际可用 OpenAI 兼容格式自定义 provider，价格也可在计费面板调整。本文记录联网核实的模型状态。

---

## DeepSeek（api.deepseek.com）

| 模型 ID | 状态 | 上下文 | 官方价（元/百万 token） |
|---------|------|--------|----------------------|
| `deepseek-v4-flash` | ✅ 正式版（V4-Flash-0731） | 1M | 输入1 / 输出2 / 缓存0.2 |
| `deepseek-v4-pro` | ⚠️ 正式版未发布（预览中） | 1M | 原价 输入12/输出24/缓存1；曾限时2.5折 输入3/输出6/缓存0.25 |

- 两者共享 OpenAI 兼容 API，仅 model 字段不同
- **注意**：Goink 内置默认价格（输入1.35/输出8.1/缓存0.27）是**初始参考值**，需在计费面板按实际订阅调整
- 2026-07 起 DeepSeek 落地**峰谷分时计费**：工作日 9-12、14-18 点价格翻倍，夜间/周末平价

## GLM（open.bigmodel.cn）

| 模型 ID | 状态 | 上下文 |
|---------|------|--------|
| `glm-5.2` | ✅ 最新（已加入内置） | 1M |
| `glm-5.1` / `glm-5` / `glm-5-turbo` | ✅ 可用 | 200K |
| `glm-4.6` / `glm-4.7` | ⚠️ 2026-10-10 下架（已从内置移除） | — |

## Qwen（dashscope）

| 模型 ID | 状态 | 上下文 |
|---------|------|--------|
| `qwen3.7-max` / `qwen3.7-plus` / `qwen3.7-flash` | ✅ 最新（flash 已补入） | 1M |
| `qwen3.6-plus` / `qwen3.6-flash` | ✅ 可用 | 1M |

- GLM 下架官方推荐转 qwen3.7-plus/max/flash

## Kimi（api.moonshot.cn）

| 模型 ID | 状态 | 上下文 |
|---------|------|--------|
| `kimi-k2.7-code` | ✅ | 256K（固定 temperature=1.0，仅思考模式不可关） |
| `kimi-k2.6` / `kimi-k2.5` | ✅ | 256K |

- k2.7-code 传入非 1.0 的 temperature 会报错（Goink 的 moonshotBuildRequest 已处理：删除 temperature/reasoning_effort）

## 技术依赖版本

| 依赖 | 当前 | 最新 | 建议 |
|------|------|------|------|
| `sqlite-vec-go-bindings` | v0.1.6 | v0.1.7（2026-03） | 暂不升（v0.1.6 稳定） |
| ONNX Runtime | 运行时下载 | 季度更新 | 保持 |

---

## 维护建议

1. 内置 provider 模板是**默认参考**，用户主用 OpenAI 兼容格式时不受影响
2. 模型 ID 失效（如下架）会让直接选用的用户报错——**下架前应在内置移除**
3. 价格以计费面板用户配置为准，内置默认值仅初始化
4. 每次打包前建议联网核对 `providers.go` 模型列表
