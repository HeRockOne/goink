package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"novel/internal/llm"
)

// InitRunningTokens 对初始消息列表逐条 token 计数，返回按 role 的分组统计。
func (a *Agent) InitRunningTokens(messages []map[string]any) map[string]int {
	tokens := map[string]int{"system": 0, "user": 0, "assistant": 0, "tool": 0}
	for _, m := range messages {
		role, _ := m["role"].(string)
		if _, ok := tokens[role]; ok {
			n, err := llm.CountMessageTokens(m)
			if err != nil {
				a.logger.Warn("token count failed", "role", role, "err", err)
			}
			tokens[role] += n
		}
	}
	return tokens
}

// updateUsage 计算 usage_ratio + 分角色 detail → 持久化到 session + 推送前端。
// 缓存命中 token 做 session 级累计，每次请求累加到历史值上。
// usage_ratio 用本地估算（runningTokens + 固定前缀 + 工具定义），单调递增不回跳，
// 避免 provider 当轮 total（含当轮输出大小波动）导致占用显示忽大忽小。
// 主/子 agent 的请求都计入命中率统计（同一模型、真实成本口径），消息级审计按 agent_type 分开写，
// 避免同 turn 内互相覆盖。前端事件只由主 agent 推送（子 agent 运行期间占用显示保持主会话值）。
func (a *Agent) updateUsage(ctx context.Context, apiUsage map[string]any, runningTokens map[string]int, toolTokens int, opts RunOptions) {
	apiTotal, _ := apiUsage["total_tokens"].(float64)

	agentType := opts.AgentType
	if agentType == "" {
		agentType = "main"
	}
	isMain := agentType == "main"

	// 分角色 detail：
	// - system 用固定前缀的精确值（首轮写入 session.extra_metadata.fixed_prefix_tokens），
	//   不含动态 system 消息（phase reminder 等，量小归入格式开销）
	// - user/assistant/tool 用 runningTokens 本地计数（tiktoken 估算，量级准确）
	// - 差值（工具定义已计入固定前缀；剩余为消息格式开销 + 动态 system）单独展示为 overhead
	fixedPrefix := 0
	if sess, err := a.session.GetSession(ctx, opts.SessionID); err == nil && sess.ExtraMetadata != "" {
		var meta map[string]any
		if json.Unmarshal([]byte(sess.ExtraMetadata), &meta) == nil {
			if v, _ := meta["fixed_prefix_tokens"].(float64); v > 0 {
				fixedPrefix = int(v)
			}
		}
	}

	detail := make(map[string]int)
	for role, tokens := range runningTokens {
		if role == "system" && fixedPrefix > 0 {
			detail[role] = fixedPrefix
		} else {
			detail[role] = tokens
		}
	}
	detailSum := detail["system"] + detail["user"] + detail["assistant"] + detail["tool"]
	overhead := 0
	if int(apiTotal) > detailSum {
		overhead = int(apiTotal) - detailSum
	}

	// 累计 session 级缓存命中/未命中 token + 输出 token（累计值用于计费面板）
	// 按模型独立累计，支持全局合计 + 模型级明细
	// 支持两种缓存格式：
	// 1. OpenAI 标准格式：prompt_tokens_details.cached_tokens（缓存命中），miss = prompt_tokens - cached
	// 2. DeepSeek 格式：prompt_cache_hit_tokens + prompt_cache_miss_tokens（各自独立）
	accHit, accMiss := float64(0), float64(0)
	accCompletion := float64(0)
	perModel := make(map[string]map[string]float64) // modelID → {hit, miss, comp}
	if sess, err := a.session.GetSession(ctx, opts.SessionID); err == nil && sess.Usage != "" {
		var old map[string]any
		if json.Unmarshal([]byte(sess.Usage), &old) == nil {
			if v, _ := old["prompt_cache_hit_tokens"].(float64); v > 0 {
				accHit = v
			}
			if v, _ := old["prompt_cache_miss_tokens"].(float64); v > 0 {
				accMiss = v
			}
			if v, _ := old["acc_completion_tokens"].(float64); v > 0 {
				accCompletion = v
			}
			// 读取按模型累计数据
			if pm, ok := old["per_model"].(map[string]any); ok {
				for modelID, data := range pm {
					if d, ok := data.(map[string]any); ok {
						m := make(map[string]float64)
						if v, _ := d["hit"].(float64); v > 0 {
							m["hit"] = v
						}
						if v, _ := d["miss"].(float64); v > 0 {
							m["miss"] = v
						}
						if v, _ := d["comp"].(float64); v > 0 {
							m["comp"] = v
						}
						perModel[modelID] = m
					}
				}
			}
		}
	}

	// 从 API 提取缓存 token
	hitTokens, missTokens := float64(0), float64(0)
	promptTokens, _ := apiUsage["prompt_tokens"].(float64)

	// 优先尝试 OpenAI 标准格式：prompt_tokens_details.cached_tokens
	var details map[string]any
	switch d := apiUsage["prompt_tokens_details"].(type) {
	case map[string]any:
		details = d
	case string:
		json.Unmarshal([]byte(d), &details)
	}
	if details != nil {
		if cached, ok := details["cached_tokens"].(float64); ok {
			// 键存在即按 OpenAI 语义处理：miss = prompt - cached（含 cached=0 的全未命中场景，
			// 避免该轮 prompt_tokens 既不进 hit 也不进 miss，导致计费 miss 累计偏低、命中率虚高）
			hitTokens = cached
			missTokens = promptTokens - cached
			if missTokens < 0 {
				missTokens = 0
			}
		}
	}

	// Fallback 到 DeepSeek 格式
	if hitTokens == 0 && missTokens == 0 {
		if hit, ok := apiUsage["prompt_cache_hit_tokens"].(float64); ok && hit > 0 {
			hitTokens = hit
		}
		if miss, ok := apiUsage["prompt_cache_miss_tokens"].(float64); ok && miss > 0 {
			missTokens = miss
		}
	}

	accHit += hitTokens
	accMiss += missTokens
	if comp, _ := apiUsage["completion_tokens"].(float64); comp > 0 {
		accCompletion += comp
	}

	// 模型 ID（计费表 + 消息审计共用）
	modelID := ""
	if opts.Model != nil {
		modelID = opts.Model.ID
	}
	if modelID == "" {
		modelID = "unknown"
	}

	// 持久化模型级 token 到专用表（传增量值，主/子 agent 都计入计费）
	deltaComp := float64(0)
	if v, _ := apiUsage["completion_tokens"].(float64); v > 0 {
		deltaComp = v
	}
	if err := a.session.UpsertModelUsage(ctx, opts.SessionID, modelID, hitTokens, missTokens, deltaComp); err != nil {
		a.logger.Warn("保存模型 usage 失败", "model", modelID, "err", err)
	}

	// 更新 per_model 累计（主/子 agent 统一累计，真实成本口径）
	m := perModel[modelID]
	if m == nil {
		m = map[string]float64{"hit": 0, "miss": 0, "comp": 0}
		perModel[modelID] = m
	}
	m["hit"] += hitTokens
	m["miss"] += missTokens
	if deltaComp > 0 {
		m["comp"] += deltaComp
	}

	// 保存 API 返回的精确 usage 到各自 agent 的 assistant 消息（持久化审计 + 趋势面板口径）。
	// 按 agent_type 分开定位，主/子 agent 互不覆盖
	if err := a.session.UpdateMessageUsage(ctx, opts.SessionID, opts.TurnID, agentType, map[string]any{
		"prompt_tokens":     apiUsage["prompt_tokens"],
		"completion_tokens": apiUsage["completion_tokens"],
		"total_tokens":      apiUsage["total_tokens"],
		"cached_tokens":     hitTokens,
		"model":             modelID,
	}); err != nil {
		a.logger.Debug("保存消息 usage 失败（该 agent 无 assistant 消息）", "err", err)
	}

	usage := map[string]any{
		"prompt_tokens":            apiUsage["prompt_tokens"],
		"completion_tokens":        apiUsage["completion_tokens"],
		"total_tokens":             apiUsage["total_tokens"],
		"prompt_cache_hit_tokens":  accHit,
		"prompt_cache_miss_tokens": accMiss,
		"acc_completion_tokens":    accCompletion,
		"per_model":                perModel,
		"context_window":           opts.Model.ContextWindow,
		"running_tokens":           detail,
		"detail":                   detail,
		"detail_is_estimate":       true, // 分角色仅 system 精确（固定前缀），其余为 tiktoken 估算
		"overhead_tokens":          overhead,
	}

	a.logger.Info("usage 推送",
		"session", opts.SessionID,
		"turn", opts.TurnID,
		"agent_type", agentType,
		"model", modelID,
		"accComp", accCompletion,
		"perModel", fmt.Sprintf("%+v", perModel))

	// 偶发全 miss 告警：上轮命中、本轮 hit=0 且 miss 大 → 厂商侧缓存失效
	// （MiniMax 被动缓存"根据系统负载自动调整过期时间"，负载驱逐/路由重分配）
	// 记录便于统计频率，区分代码问题与厂商行为
	if hitTokens == 0 && missTokens > 10000 && isMain {
		a.logger.Warn("全量缓存未命中（厂商侧缓存失效或路由变化）",
			"session", opts.SessionID,
			"turn", opts.TurnID,
			"miss_tokens", missTokens,
			"prompt_tokens", promptTokens)
	}

	// usage_ratio 用本地估算（runningTokens + 固定前缀 + 工具定义），单调递增不回跳，
	// 避免 provider 当轮 total（含当轮输出大小波动）导致占用显示忽大忽小
	if opts.Model.ContextWindow > 0 {
		usedTokens := sumRunningTokens(runningTokens) + fixedPrefix + toolTokens
		usage["usage_ratio"] = float64(usedTokens) / float64(opts.Model.ContextWindow) * 100
	}
	if accHit+accMiss > 0 {
		usage["cache_hit_ratio"] = accHit / (accHit + accMiss) * 100
	}

	// 持久化 session.Usage（主/子 agent 统一累计，重启后显示一致）
	if b, err := json.Marshal(usage); err == nil {
		if err := a.session.UpdateSessionUsage(ctx, opts.SessionID, string(b)); err != nil {
			a.logger.Warn("持久化 usage 失败", "err", err)
		}
	}

	// 前端事件只由主 agent 推送：子 agent 运行期间占用显示保持主会话值，避免跳动
	if !isMain {
		return
	}

	wails.EventsEmit(ctx, "agent:"+strconv.Itoa(opts.TurnID), AgentEvent{
		TurnID:    opts.TurnID,
		Type:      EventUsage,
		Usage:     usage,
		Timestamp: time.Now(),
	})
}
