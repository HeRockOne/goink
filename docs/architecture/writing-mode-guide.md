# 创作模式决策与质量底线成本区间

> 2026-08-16 建立。数据口径：cacheprobe 规范基准（严格门禁流程，low 档思考，
> DeepSeek 价 0.02/1/2 元/百万 token）。价格可调：`go run ./cmd/cacheprobe -cache X -input X -output X table`。
> 真机按设置页价格换算（ContextRing 同源：price_input/price_output/cache_price）。

## 一、成本区间速查（规范基准，每章成本）

| 模式 | 成本 ¥/章 | 相对倍数 | 适用场景 |
|------|----------|---------|---------|
| 批量 10-60 章 | 0.034 - 0.040 | 1.0× | 长卷连续创作（≥10 章），配三章一轮自检 |
| 批量 5 章 | 0.055 | 1.4× | 常规批量（5 章），配三章一轮自检 + 批末全批审稿 |
| 续写批量 5 章（历史 4 章） | 0.098 | 2.5× | 已有历史的会话继续批量写（历史轮成本已发生） |
| 单章 1 轮（新会话） | 0.182 | 4.6× | 关键章节、开书、大爽点、卷末 |
| 单章 3-5 轮 | 0.222 - 0.258 | 5.6-6.5× | 连续单章精写 |
| 续写单章（历史 3 章） | 0.261 | 6.6× | 已有历史的会话单独写一章 |
| 混合 3+2+3 / 5+5+5 | 0.198 - 0.223 | 5.0-5.6× | 边写边调整设定（质量上限最高，成本中等偏上） |

**成本三定律**：
1. 批量永远比单章便宜（固定成本摊薄 3-5 倍）
2. 续写永远比新会话便宜（前缀复用，miss 只加新增）——但"续写单章"仍是单章固定成本，不划算
3. 质量节奏几乎不花钱（三章一轮自检 +0.3%，批末全批审稿 +4%）

## 二、模式决策树

```
要写多少章？
├─ 1 章（关键章节/开书/大爽点/卷末/设定剧变）→ 单章完整门禁（¥0.18-0.26/章）
├─ 3-5 章 → 批量 + 完整门禁流程（每章独立 review+maintain，¥0.11/章）
└─ ≥6 章 → 批量 + 三章一轮自检 + 批末全批审稿（¥0.04-0.10/章）★ 推荐
     └─ 会话已有历史？→ 直接续写（¥0.098/章），别开新会话重读设定
```

**推荐主路径**：批量 5-10 章 + 三章一轮自检 + 批末全批审稿（batchLightEndReview）。
质量 9.0（与单章持平）、成本 ¥0.055-0.098/章（省 55-70%）。

**单章模式只留给**：开书、卷首/卷末、大爽点、设定剧变、需要反复打磨的章节。

## 三、质量底线（三道硬约束，任何模式下不得低于）

| 机制 | 触发点 | 防什么 |
|------|--------|--------|
| check_story_consistency（写时把关） | 每章 write 转出前，SQL 实证核对 | 伏笔超期、角色断档、物品冲突、死者复出 |
| 三章一轮状态对照自检 | 批量每 3 章边界 | 设定矛盾（前文死角色后文复出）、文风偏移、章节混乱 |
| 审稿覆盖核验 | 批末子代理审稿后 | 审稿漏章（报告必须列实际审读清单，主 agent 核对） |

辅助底线：每章 miniMaintain 六件套硬约束（create_scene/update_character/create_timeline_entry/
update_timeline_entry/create_item_occurrence/update_writing_snapshot 缺件拒绝声明下一章）。

## 四、真机核查：用规范基准反查质量底线

真机单章/批量成本与规范基准对比，偏差方向 = 质量健康度：

| 真机成本 vs 基准 | 含义 | 处置 |
|------------------|------|------|
| 显著低于基准（如批量 <¥0.07/章） | LLM 偷工减料：miniMaintain 缺件、跳过大纲、跳过自检 | 查 goink.log 门禁拦截/require 记录；检查 messages 表工具调用是否覆盖六件套；必要时换模型 |
| 在基准 ±20% 内 | 正常 | - |
| 显著高于基准（如 >¥0.15/章 批量） | LLM 发癫：思考重复循环、超长思考、拖沓重试 | 查 messages 表 content 中 <think> 重复/超长消息（>5K 字符即异常）；换模型或换直连 |

**核查命令**（PowerShell，sqlite3 在 Android SDK platform-tools）：

```powershell
$sq = 'D:\AndroidBuild\sdk\platform-tools\sqlite3.exe'
# 会话总成本三要素（权威口径）
& $sq 'D:\Goink\novel-agent.db' "SELECT model_id, hit_tokens, miss_tokens, completion_tokens FROM model_usage WHERE session_id='<会话ID>';"
# 抽风检测：content 超长消息（>5K 字符 = 超长思考/重复循环嫌疑）
& $sq 'D:\Goink\novel-agent.db' "SELECT id, turn_id, length(content) FROM messages WHERE session_id='<会话ID>' AND role='assistant' AND length(content) > 5000;"
# miniMaintain 覆盖核验：该会话用过的工具清单
& $sq 'D:\Goink\novel-agent.db' "SELECT json_extract(extra_metadata,'$.tool_calls[0].function.name') as tool, count(*) FROM messages WHERE session_id='<会话ID>' AND extra_metadata LIKE '%tool_calls%' GROUP BY tool ORDER BY 2 DESC;"
```

## 五、模拟器使用

- `go run ./cmd/cacheprobe table`：14+2 场景成本表（含续写单章/续写批量）
- `go run ./cmd/cacheprobe -effort high table`：思考密集档（mimo high 会话对标）
- Goink 设置 → 写书成本模拟：推理深度下拉 + 续写场景（历史章输入）
- 模拟器是规范基准：只预测"严格按门禁做该花多少"，LLM 偷工减料/发癫不建模

## 六、口径说明与已知边界

- 价格为 DeepSeek 价（0.02/1/2）；mimo/yjm 等按设置页价格换算，相对倍数不变
- 批量 40 章以上 ≈ 1M 窗口 × 0.7 压缩阈值边缘，真机会触发压缩（成本跳升），模拟器不建模压缩
- 规范基准不含：首轮被动缓存（-firsthit 默认 0.84，DeepSeek 可设 0）、mimo 每轮新增固定内容被动缓存（命中率差 2-3pp）
- 真机批量会话可能含抽风思考（曾占输出 58%），对标前先剔除
