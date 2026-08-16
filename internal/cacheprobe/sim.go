// cacheprobe 缓存命中率字节级探针（无网络、无 LLM 调用、无需 API Key）。
//
// 原理：DeepSeek/商汤等提供"字节精确前缀匹配"的磁盘缓存（官方文档确认：
// 命中 = 本次请求与上次请求的字节级公共前缀，TTL 内有效）。探针用真实的
// 消息序列化（Go map + encoding/json，键序确定）构造每次 LLM 调用请求，
// 按字节公共前缀计算理论命中量，对照两种 NovelState 注入协议：
//
//	go run ./cmd/cacheprobe now      # 修复后：NS 落库（本 repo 当前行为）
//	go run ./cmd/cacheprobe legacy   # 修复前：NS 动态注入不落库（旧行为）
//
// 两个场景：短问答 5 轮、门禁创作 5 轮（prepare→outline→write→review→maintain
// 每轮 20 次工具调用）。输出每次调用的 hit/miss 与累计命中率。
// 折算：1 token ≈ 4 字节（中文约 3-4 字节/字，近似）。
package cacheprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"novel/internal/agentcfg"
	"novel/internal/chapter"
	"novel/internal/config"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/novel"
	"novel/internal/platform"
	"novel/internal/skill"
)

// ---- 消息级缓存模拟器（复刻 provider 语义 + tiktoken 精确计数） ----

// TokenCache 以"消息"为单位模拟前缀缓存：
// - 连续性判定：消息序列的字节级公共前缀（精确，消息内容相同则字节相同）
// - token 统计：每条消息用 tiktoken 精确计数（llm.CountMessageTokens），
//   命中 = 公共前缀覆盖的消息 token 和，miss = 其余消息 token 和
// - tools 定义作为第一条固定前缀消息参与计数
type TokenCache struct {
	prevBytes []byte // 上次请求的完整字节（连续性判定）
	prevMsgs  []map[string]any
	// 增量复用：上次请求的消息 token 累计与消息数组字节截止位置
	// （消息 append-only 是模拟器不变量：output diff 同样依赖它；
	// transform 是唯一破坏者，增量路径已排除）
	prevMsgsN   int64
	prevMsgEnd  int
	hit         int64 // token
	miss        int64 // token
	output      int64 // LLM 输出 token（新增 assistant 消息，与输入侧同源）
	// transform 发送前变换（clean 方案用）：每次 Step 前对完整消息序列做处理，
	// 如把已消费的 skill 全文替换为占位符。变换后字节才参与前缀判定——
	// 这样"滑出保留窗口"是唯一前缀断裂点，连续性好。
	transform func([]map[string]any) []map[string]any
	// clearedIDs 阶段切换清理用：已结束阶段的 read/read_required 调用 ID 集合。
	// 发送前 transform 按此集合替换为占位符（当前阶段 read 保留全文）。
	clearedIDs map[string]bool
	// missCat 诊断钩子：非 nil 时按分类累计 miss token（与 miss 计算同路径）。
	missCat   func(m map[string]any) string
	MissByCat map[string]int64
	// 窗口刻度打点（RunWindowCost 用）
	marks     []WindowMark
	reqCount  int
	lastTotal int64 // 最近一次请求的输入大小（最终历史规模，刻度表终点用）
	peakTotal int64 // 窗口内历史峰值（跨压缩单调不减，刻度打点用：压缩重建不清零）
	// 阶段打点（混合模式用）：按创作阶段边界记录，非阈值
	stageMarks []StageMark
	// 上下文压缩建模（真机 agent.go:434-455 对齐）：每轮开始时
	// (runningTokens+toolTokens)/ContextWindow >= threshold 触发压缩重建。
	// 触发后调用 onCompress（由会话构造方提供：重置链 + 重建消息序列），
	// compressCount 累计触发次数（诊断/刻度表展示用）。
	contextWindow int
	compressTh    float64
	onCompress    func()
	compressCount int
}

// StageMark 阶段打点快照：混合模式每个创作阶段（开书/短对话/单章轮/批量轮）
// 结束时记录累计状态，回答"混合模式每阶段花了多少钱、写了多少章"。
type StageMark struct {
	Stage    string // 阶段名："开书完成" / "短对话 N" / "单章 N" / "批量轮 N"
	Chapter  int    // 到达时累计写到第几章
	Hit      int64
	Miss     int64
	Out      int64
	Requests int
	Total    int64 // 当前历史大小（最后请求输入）
}

// RecordStage 阶段边界打点（buildMixedSessionCore 每阶段结束调用）。
// stageMarks 为 nil（single/batch 模式）时跳过。
func (c *TokenCache) RecordStage(stage string) {
	if c.stageMarks == nil {
		return
	}
	c.stageMarks = append(c.stageMarks, StageMark{
		Stage:    stage,
		Chapter:  simCurrentChapter,
		Hit:      c.hit,
		Miss:     c.miss,
		Out:      c.output,
		Requests: c.reqCount,
		Total:    c.lastTotal,
	})
}

// SetMissCat 启用 miss 分类统计（诊断用，不影响模拟结果）。
func (c *TokenCache) SetMissCat(fn func(m map[string]any) string) {
	c.missCat = fn
	c.MissByCat = map[string]int64{}
}

// ResetMissByCat 清空 miss 分类累计（续写场景：历史构建轮的 miss 不计入目标统计）。
func (c *TokenCache) ResetMissByCat() {
	if c.MissByCat != nil {
		c.MissByCat = map[string]int64{}
	}
}

// MarkCleared 标记 tool_call_id 为可清理（阶段切换时调用：上一阶段的 read 结果）。
func (c *TokenCache) MarkCleared(ids ...string) {
	if c.clearedIDs == nil {
		c.clearedIDs = make(map[string]bool)
	}
	for _, id := range ids {
		c.clearedIDs[id] = true
	}
}

// msgFingerprint 消息的轻量稳定指纹（缓存 key 用）。
// 消息字段全集：role/content/name/tool_call_id/reasoning_content/tool_calls。
// 不 marshal——直接拼接字符串；内容不同则指纹必不同（content 完整拼入）。
// 旧实现用 json.Marshal 结果做 key，导致每次请求即使缓存命中也要对全部历史消息
// 重新序列化（600 请求 × 300+ 历史消息 = 18 万次 marshal，profile 显示占 28% CPU）。
func msgFingerprint(m map[string]any) string {
	var b strings.Builder
	if v, ok := m["role"].(string); ok {
		b.WriteString(v)
	}
	b.WriteByte(0)
	if v, ok := m["content"].(string); ok {
		b.WriteString(v)
	}
	b.WriteByte(0)
	if v, ok := m["name"].(string); ok {
		b.WriteString(v)
	}
	b.WriteByte(0)
	if v, ok := m["tool_call_id"].(string); ok {
		b.WriteString(v)
	}
	b.WriteByte(0)
	if v, ok := m["reasoning_content"].(string); ok {
		b.WriteString(v)
	}
	if tcs, ok := m["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			if tcm, ok := tc.(map[string]any); ok {
				b.WriteByte(0)
				if fn, ok := tcm["function"].(map[string]any); ok {
					if v, ok := fn["name"].(string); ok {
						b.WriteString(v)
					}
					b.WriteByte(0)
					if v, ok := fn["arguments"].(string); ok {
						b.WriteString(v)
					}
				}
			}
		}
	}
	return b.String()
}

// msgTokens 计算单条消息的精确 token 数（tool_calls/tool_call_id/reasoning 计入）。
// 消息内容不变则 token 数不变——用缓存避免每条历史消息被重复 tiktoken 编码（性能关键：
// 400+ 次请求 × 上千条历史消息，不缓存约 8 万次 Encode，缓存后每个唯一消息只算一次）。
var msgTokenCache sync.Map // key: msgFingerprint → int token 数

func msgTokens(m map[string]any) int {
	key := msgFingerprint(m)
	if v, ok := msgTokenCache.Load(key); ok {
		return v.(int)
	}
	n, err := llm.CountMessageTokens(m)
	if err != nil {
		return 0
	}
	msgTokenCache.Store(key, n)
	return n
}

// toolDefs 序列化缓存（工具定义不变，避免每次请求重复 marshal + 编码）。
var (
	toolsJSONOnce    sync.Once
	toolsJSONCached  []byte
	toolsTokenCached int64
)

func cachedToolsJSON() ([]byte, int64) {
	toolsJSONOnce.Do(func() {
		toolsJSONCached, _ = json.Marshal(toolDefs)
		n, _ := llm.CountTokens(string(toolsJSONCached))
		toolsTokenCached = int64(n)
	})
	return toolsJSONCached, toolsTokenCached
}
func NewTokenCache() *TokenCache { return &TokenCache{} }

// clearPlaceholderPrefix 已清理占位符前缀（clean 方案已从运行时移除，模拟保留作对照研究）。
const clearPlaceholderPrefix = "[已读技能内容已清理: "

// WithCleanTransform 启用发送前清理变换：把 read/read_required 的 skill 全文
// 替换为占位符，仅保留最近 retain 条完整结果（retain<0 = 不清理）。
// 变换在 Step 内对完整请求执行（含 history+cur），保留窗口滑动时前缀断裂一次。
func NewCleanCache(retain int) *TokenCache {
	c := &TokenCache{}
	if retain >= 0 {
		c.transform = func(msgs []map[string]any) []map[string]any {
			return cleanVersion(msgs, retain)
		}
	}
	return c
}

// NewPhaseCleanCache 阶段切换清理：只在 set_phase 切换时标记上一阶段 read 的调用 ID
// 为可清理（MarkCleared），发送前替换为占位符；当前阶段 read 保留全文。
// 对齐真实实现（agent 在 set_phase 成功时标记 clearedIDs）。
func NewPhaseCleanCache() *TokenCache {
	c := &TokenCache{}
	c.transform = func(msgs []map[string]any) []map[string]any {
		return phaseCleanVersion(msgs, c.clearedIDs)
	}
	return c
}

// phaseCleanVersion 发送前变换：tool 消息的 tool_call_id 在 clearedIDs 中 → 占位符。
// 只替换 read/read_required 的结果；当前阶段（未标记）保留全文。
func phaseCleanVersion(messages []map[string]any, cleared map[string]bool) []map[string]any {
	if len(cleared) == 0 || len(messages) == 0 {
		return messages
	}
	out := make([]map[string]any, len(messages))
	copy(out, messages)
	for i, m := range messages {
		role, _ := m["role"].(string)
		if role != "tool" {
			continue
		}
		name, _ := m["name"].(string)
		if name != "auto_skill_injection" && name != "read_required" && name != "read" {
			continue
		}
		id, _ := m["tool_call_id"].(string)
		if !cleared[id] {
			continue
		}
		dup := make(map[string]any, len(m))
		for k, v := range m {
			dup[k] = v
		}
		dup["content"] = clearPlaceholderPrefix + name + "]"
		out[i] = dup
	}
	return out
}


// requestTokens 计算完整请求的 token 数：tools 前缀（固定 system 消息）+ 各消息。
// 全量遍历（仅首轮/前缀断裂时使用；正常请求走 TokenCache.step 的增量路径）。
func requestTokens(messages []map[string]any) (int64, int64) {
	_, toolsN := cachedToolsJSON()
	return toolsN, requestMsgsTokens(messages)
}

// requestMsgsTokens 各消息 token 和（msgTokens 内部有指纹缓存，重复消息只编码一次）。
func requestMsgsTokens(messages []map[string]any) int64 {
	var msgsN int64
	for _, m := range messages {
		msgsN += int64(msgTokens(m))
	}
	return msgsN
}

// requestTail 请求体固定尾部（promptBytes 结构的一部分，增量字节拼接定位用）。
var requestTail = []byte(`],"model":"goink-sim","stream":true,"stream_options":{"include_usage":true},"temperature":0.7}`)

// Step 每次 LLM 调用。返回 (hit, miss) token 数。
// 连续性判定用字节公共前缀；token 统计按消息级精确计数。
// 主 Agent 请求：应用 transform（clean 方案）。
func (c *TokenCache) Step(messages []map[string]any) (int64, int64) {
	return c.step(messages, true)
}

// StepRaw 不应用 transform（子代理/压缩摘要保持全文，对齐真实实现：
// 子代理审稿需要读 skill 原文，压缩摘要需要全量上下文）。
func (c *TokenCache) StepRaw(messages []map[string]any) (int64, int64) {
	return c.step(messages, false)
}

// WindowMark 上下文规模刻度快照：单窗口内历史首次达到 threshold 时的累计成本。
type WindowMark struct {
	Threshold int64   // 刻度（token）：128K/256K/512K/1024K
	Reached   bool    // 是否到达（批量不够大可能到不了大刻度）
	Hit       int64   // 到达时的累计 hit
	Miss      int64   // 到达时的累计 miss
	Out       int64   // 到达时的累计 output
	Requests  int     // 到达时的请求数
	Chapter   int     // 到达时写到的章节号
}

// simWindowThresholds 打点刻度（RunWindowCost 设置，单线程）。
var simWindowThresholds []int64

// simCurrentChapter 当前处理章节号（批量场景每章循环更新，打点记录用）。
var simCurrentChapter int

// markWindow 检查本次请求输入（历史规模）是否跨过未到达的刻度，记录快照。
// 调用点：step() 累计更新后。
// 用 peakTotal（历史峰值）打点：压缩重建后历史缩回骨架，但窗口内曾到达的
// 峰值保留——刻度表如实反映"这个窗口内历史最长到过多大"。
// 只有挂了 marks（simWindowThresholds 非空时 runTriple 给 now cache 初始化）的 cache 打点；
// legacy/clean cache 的 marks 为 nil，直接跳过（刻度表只反映 now 协议）。
func (c *TokenCache) markWindow(total int64) {
	if c.marks == nil {
		return
	}
	if total > c.peakTotal {
		c.peakTotal = total
	}
	for i, th := range simWindowThresholds {
		if !c.marks[i].Reached && c.peakTotal >= th {
			c.marks[i] = WindowMark{
				Threshold: th,
				Reached:   true,
				Hit:       c.hit,
				Miss:      c.miss,
				Out:       c.output,
				Requests:  c.reqCount,
				Chapter:   simCurrentChapter,
			}
		}
	}
}

// SimFirstHitRatio 首轮固定前缀（tools + 系统消息）的服务端被动缓存命中率。
// 真机实测（mimo-v2.5 批量 5 章，2026-08-16）：首轮输入 34.3K 命中 28.7K（83.7%）——
// MiniMax 对固定字节前缀有服务端被动缓存；DeepSeek 磁盘缓存首轮通常不命中，
// 用 -firsthit 0 恢复保守口径。默认 0.84 贴近真机（模拟主要服务 mimo 场景）。
var SimFirstHitRatio = 0.84

func (c *TokenCache) step(messages []map[string]any, applyTransform bool) (int64, int64) {
	transformed := false
	if applyTransform && c.transform != nil {
		messages = c.transform(messages)
		transformed = true
	}
	_, toolsN := cachedToolsJSON()

	if c.prevBytes == nil {
		// 首轮全量：token 累计 + 完整字节构建
		msgsN := requestMsgsTokens(messages)
		total := toolsN + msgsN
		reqBytes := promptBytes(messages)
		if c.missCat != nil {
			for _, m := range messages {
				c.MissByCat[c.missCat(m)] += int64(msgTokens(m))
			}
			c.MissByCat["fixed"] += toolsN
		}
		c.miss += total
		// 首轮固定前缀被动缓存建模（真机 mimo 实测 ~84%）：fixed 类消息与 tools 部分命中。
		// 只影响首轮 ~30K，对长会话影响 <0.1%，对短场景（单章 1 轮/批量首轮）显著。
		hitPart := int64(0)
		if SimFirstHitRatio > 0 {
			var fixedN int64
			if c.missCat != nil {
				for _, m := range messages {
					if c.missCat(m) == "fixed" {
						fixedN += int64(msgTokens(m))
					}
				}
			}
			hitPart = int64(float64(fixedN+toolsN) * SimFirstHitRatio)
			c.hit += hitPart
			c.miss -= hitPart
			if c.missCat != nil {
				c.MissByCat["fixed"] -= hitPart
			}
		}
		c.reqCount++
		c.lastTotal = total
		c.markWindow(total)
		c.prevBytes = reqBytes
		c.prevMsgs = append([]map[string]any{}, messages...)
		c.prevMsgsN = msgsN
		c.prevMsgEnd = len(reqBytes) - len(requestTail)
		return hitPart, total - hitPart
	}

	// 与真实 buildPayload 一致：完整请求体字节用于连续性判定
	reqBytes := promptBytes(messages)
	lcp := longestCommonPrefix(c.prevBytes, reqBytes)

	// 增量路径：新请求字节完整包含上次请求（lcp 覆盖到 prevBytes 末尾）→
	// 历史消息全部命中，只对新增消息做 token 统计与字节拼接。
	// 此前每次请求都对 300+ 条历史消息重复做指纹/marshal，是 60 秒耗时的根源。
	if !transformed && lcp >= len(c.prevBytes) && len(messages) > len(c.prevMsgs) {
		newMsgs := messages[len(c.prevMsgs):]
		var msgsN int64 = c.prevMsgsN
		for _, m := range newMsgs {
			msgsN += int64(msgTokens(m))
		}
		total := toolsN + msgsN
		// 字节增量：历史部分复用 prevBytes，只在消息数组尾部插入新增消息
		var buf []byte
		buf = append(buf, c.prevBytes[:c.prevMsgEnd]...)
		for i, m := range newMsgs {
			if i > 0 || len(c.prevMsgs) > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, msgJSON(m)...)
		}
		buf = append(buf, c.prevBytes[c.prevMsgEnd:]...)

		// 命中：tools + 全部历史消息（字节前缀一致），新增消息全 miss
		hit := toolsN + c.prevMsgsN
		miss := total - hit
		if c.missCat != nil {
			for _, m := range newMsgs {
				c.MissByCat[c.missCat(m)] += int64(msgTokens(m))
			}
		}
		c.hit += hit
		c.miss += miss
		c.reqCount++
		c.lastTotal = total
		c.markWindow(total)
		for _, m := range newMsgs {
			if role, _ := m["role"].(string); role == "assistant" {
				c.output += int64(msgTokens(m))
			}
		}
		c.prevBytes = buf
		c.prevMsgs = append([]map[string]any{}, messages...)
		c.prevMsgsN = msgsN
		c.prevMsgEnd = len(buf) - len(requestTail)
		return hit, miss
	}

	// 全量路径（首轮之外：前缀断裂——子代理 fork/auto 剔除 NS/transform 内容替换）
	msgsN := requestMsgsTokens(messages)
	total := toolsN + msgsN

	// 工具定义在最前（固定前缀）：只要 lcp 覆盖到 tools 前缀，tools 即命中。
	// 前缀：{"tools":<toolsJSON>,"model":"goink-sim","messages":[
	toolsJSON, _ := json.Marshal(toolDefs)
	toolsPrefix := []byte(`{"tools":`)
	toolsPrefix = append(toolsPrefix, toolsJSON...)
	// toolsN 命中：lcp 至少覆盖到 tools 前缀之后（含 tools 本身）
	hitMsgs := int64(0)
	if lcp >= len(toolsPrefix) {
		hitMsgs += toolsN
	} else if c.missCat != nil {
		// tools 未命中（罕见：工具定义变更/前缀断裂在 tools 内）→ 计入 fixed
		c.MissByCat["fixed"] += toolsN
	}
	// 消息前缀：tools 前缀 + `,"max_tokens":8192,"messages":[`
	msgPrefix := append(append([]byte{}, toolsPrefix...), []byte(`,"max_tokens":8192,"messages":[`)...)
	acc := len(msgPrefix)
	missFrom := 0 // 第一个未命中消息的索引（诊断用）
	if acc > lcp {
		// 连消息前缀都没完全命中（正常不会发生，前缀固定）
		// tools 已计入，消息不命中
	} else {
		for idx, m := range messages {
			b := msgJSON(m) // 复用消息 JSON 缓存（避免重复 marshal）
			acc += 1 + len(b)
			if acc > lcp {
				missFrom = idx
				break
			}
			hitMsgs += int64(msgTokens(m))
		}
	}

	// miss 分类统计（诊断钩子）：与 miss 计算同路径，按未命中消息分类累计
	if c.missCat != nil && missFrom < len(messages) {
		for _, m := range messages[missFrom:] {
			cat := c.missCat(m)
			c.MissByCat[cat] += int64(msgTokens(m))
		}
	}

	hit := hitMsgs
	miss := total - hit
	c.hit += hit
	c.miss += miss
	c.reqCount++
	c.lastTotal = total
	// 打点：本次请求输入跨过刻度时记录累计快照
	c.markWindow(total)
	// output 累计：相对上次请求新增的 assistant 消息 = 本次 LLM 调用的输出字节。
	// 与输入侧同源：同一段字节既作为历史输入（前缀命中判定）也作为输出计费，
	// 覆盖正文（edit arguments）、文本回答、子代理报告、thinking（reasoning_content）。
	// 消息 append-only 追加，transform（clean 占位符）不改变条数，diff 安全。
	if c.prevMsgs != nil && len(messages) > len(c.prevMsgs) {
		for _, m := range messages[len(c.prevMsgs):] {
			if role, _ := m["role"].(string); role == "assistant" {
				c.output += int64(msgTokens(m))
			}
		}
	}
	c.prevBytes = reqBytes
	c.prevMsgs = append([]map[string]any{}, messages...)
	c.prevMsgsN = msgsN
	c.prevMsgEnd = len(reqBytes) - len(requestTail)
	return hit, miss
}

// Reset 压缩重建：丢弃整个链，此后首次调用全 miss。
func (c *TokenCache) Reset() {
	c.prevBytes = nil
	c.prevMsgs = nil
	c.prevMsgsN = 0
	c.prevMsgEnd = 0
	c.hit = 0
	c.miss = 0
	c.output = 0
	if c.MissByCat != nil {
		c.MissByCat = map[string]int64{}
	}
}

// ResetChain 压缩链重建：只清前缀链（下轮首请求全 miss），保留累计统计
// （hit/miss/output/reqCount——真机压缩只重建消息，usage 照常累计）。
func (c *TokenCache) ResetChain() {
	c.prevBytes = nil
	c.prevMsgs = nil
	c.prevMsgsN = 0
	c.prevMsgEnd = 0
}

// EnableCompression 启用上下文压缩建模（对齐真机 agent.go:434-455）：
// contextWindow = 模型上下文窗口（token），threshold = 触发比例（默认 0.7）。
// onCompress 在触发时调用（由会话构造方重建消息序列）。
func (c *TokenCache) EnableCompression(contextWindow int, threshold float64, onCompress func()) {
	c.contextWindow = contextWindow
	c.compressTh = threshold
	if c.compressTh <= 0 || c.compressTh >= 1 {
		c.compressTh = 0.7
	}
	c.onCompress = onCompress
}

// MaybeCompress 每轮开始时检查 token 预算（对齐真机：runningTokens+toolTokens 占比超阈值触发）。
// 返回是否触发压缩。lastTotal 含 tools（requestTokens 口径），等价真机 runningTokens+toolTokens。
func (c *TokenCache) MaybeCompress() bool {
	if c.contextWindow <= 0 || c.onCompress == nil {
		return false
	}
	ratio := float64(c.lastTotal) / float64(c.contextWindow)
	if ratio < c.compressTh {
		return false
	}
	c.compressCount++
	c.onCompress()
	return true
}
func (c *TokenCache) TotalTokens() int64 { return c.hit + c.miss }

// Output 返回累计的 LLM 输出 token（assistant 消息字节）。
func (c *TokenCache) Output() int64 { return c.output }

func longestCommonPrefix(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// missCatOf 按消息来源分类（诊断/table 用，与 TokenCache 的 miss 计算同路径）。
// 分类:skill_inject(阶段技能注入 system)/thinking(assistant reasoning_content)
//       body(正文 edit)/outline(大纲 edit)/query(get_*/search_*)/update(create_*/update_*)
//       fixed(固定前缀+NS)/other
// 注意：assistant 消息按 tool_calls 的【工具名】逐条判定（并行组混工具时整条消息归
// 第一个命中的类——正文/大纲/查询的 token 可能互相污染，但 miss 总量不受影响）。
func missCatOf(m map[string]any) string {
	role, _ := m["role"].(string)
	if role == "system" {
		content, _ := m["content"].(string)
		if strings.HasPrefix(content, "--- ") {
			return "skill_inject"
		}
		return "fixed"
	}
	if role == "assistant" {
		// 工具调用消息的 arguments 含正文/大纲全文（LLM 输出），优先于 thinking 分类。
		// 按工具名 + 路径判定：edit chapters/ → body，edit outlines/ → outline。
		if tcs, ok := m["tool_calls"].([]any); ok {
			for _, tc := range tcs {
				tcm, _ := tc.(map[string]any)
				fn, _ := tcm["function"].(map[string]any)
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				if name == "edit" && strings.Contains(args, "chapters/") {
					return "body"
				}
				if name == "edit" && strings.Contains(args, "outlines/") {
					return "outline"
				}
			}
		}
		if rc, ok := m["reasoning_content"].(string); ok && len(rc) > 0 {
			return "thinking"
		}
		return "assistant"
	}
	name, _ := m["name"].(string)
	if name == "edit" {
		content, _ := m["content"].(string)
		if len(content) > 100 {
			return "body"
		}
		return "outline"
	}
	if strings.HasPrefix(name, "get_") || strings.HasPrefix(name, "search_") {
		return "query"
	}
	if strings.HasPrefix(name, "create_") || strings.HasPrefix(name, "update_") || name == "set_phase" || name == "read" || name == "read_required" || name == "auto_skill_injection" {
		return "update"
	}
	return "other"
}

// ---- 请求序列化（模拟服务端解析后的 token 前缀顺序） ----

var toolDefs []map[string]any

// simRegistry 全局工具注册表（initTools 构造），真实工具执行（realToolResult）用。
var simRegistry *mcp_tools.Registry

// simStore 全局技能存储（loadSystemTexts 构造），readSkill/readRequired 从它取技能正文。
var simStore *skill.Store

func initTools() {
	r := mcp_tools.NewRegistry(slog.New(slog.DiscardHandler))
	mcp_tools.RegisterAllTools(r)
	simRegistry = r
	toolDefs = r.OpenAI(nil)
	initBody()
	loadSystemTexts()
}

// skillContent 从全局 SkillStore 读取技能正文（与真实 read/read_required 工具行为一致：
// 三层查找 novel > user > builtin）。Store 不可用时回退仓库文件（仅开发环境）。
func skillContent(name string) string {
	if simStore != nil {
		if sk, ok := simStore.Get(0, name); ok {
			return sk.RawContent
		}
	}
	if p, err := filepath.Abs(filepath.Join(repoRoot(), "internal", "skill", "builtin", name+".md")); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return "（技能内容不可用: " + name + "）"
}

// promptBytes 模拟服务端把请求解析成 token 前缀后的顺序：
// tools 定义（转 system 前缀）→ 各消息顺序追加。新增消息在末尾，
// 前缀连续（与 DeepSeek/OpenAI 的 KV cache 前缀匹配语义一致）。
// msgJSONCache 缓存单条消息的序列化结果（历史消息 marshal 结果不变，避免重复序列化）。
var msgJSONCache sync.Map // key: msgFingerprint → []byte（marshal 结果）

func msgJSON(m map[string]any) []byte {
	key := msgFingerprint(m)
	if v, ok := msgJSONCache.Load(key); ok {
		return v.([]byte)
	}
	b0, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	msgJSONCache.Store(key, b0)
	return b0
}

func promptBytes(messages []map[string]any) []byte {
	toolsJSON, _ := cachedToolsJSON()
	var buf bytes.Buffer
	// 与真实 marshalPayload 完全一致：工具定义在最前，其余字段字母序
	// {"tools":<tools>,"max_tokens":8192,"messages":[...],"model":"goink-sim","stream":true,"stream_options":{"include_usage":true},"temperature":0.7}
	buf.WriteString(`{"tools":`)
	buf.Write(toolsJSON)
	buf.WriteString(`,"max_tokens":8192,"messages":[`)
	for i, m := range messages {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(msgJSON(m))
	}
	buf.WriteString(`],"model":"goink-sim","stream":true,"stream_options":{"include_usage":true},"temperature":0.7}`)
	return buf.Bytes()
}

// ---- 消息构造 ----

func sysMsg(content string) map[string]any {
	return map[string]any{"role": "system", "content": content}
}
func userMsg(content string) map[string]any {
	return map[string]any{"role": "user", "content": content}
}
// asstToolCall 构造 assistant 工具调用消息，对齐真实 ToAPIFormat：
// 恒有 reasoning_content（思考模式开启时非空），无 tool_displays（真实 API 不发送）。
func asstToolCall(id, name, args string) map[string]any {
	return map[string]any{
		"role":              "assistant",
		"content":           "",
		"reasoning_content": thinkingText(thinkingForPhase(simPhase)),
		"tool_calls": []any{map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": args},
		}},
	}
}

// asstToolCalls 构造一次请求多个并行工具调用的 assistant 消息（1 次 thinking 覆盖全部调用，
// 与真机 LLM 并行 tool_calls 行为一致——8/8 日志 19:24 一批 9 个查询 2 次请求）。
func asstToolCalls(ids, names, argsList []string) map[string]any {
	calls := make([]any, 0, len(ids))
	for i := range ids {
		calls = append(calls, map[string]any{
			"id": ids[i], "type": "function",
			"function": map[string]any{"name": names[i], "arguments": argsList[i]},
		})
	}
	return map[string]any{
		"role":              "assistant",
		"content":           "",
		"reasoning_content": thinkingText(thinkingForPhase(simPhase)),
		"tool_calls":        calls,
	}
}

// asstText 构造 assistant 纯文本消息（思考模式开启时同样带 reasoning_content）。
func asstText(content string) map[string]any {
	return map[string]any{
		"role":              "assistant",
		"content":           content,
		"reasoning_content": thinkingText(thinkingForPhase(simPhase)),
	}
}

func toolMsg(id, name, content string) map[string]any {
	return map[string]any{
		"role": "tool", "tool_call_id": id, "name": name, "content": content,
	}
}

// simPhase 当前模拟阶段（决定 assistant 消息 reasoning_content 长度）。
// 初始为开书阶段，处理 set_phase 时更新（真实：模型每次输出都带思考，长度随阶段不同）。
var simPhase = "init"

// simEffort 当前模拟的 reasoning effort 档位（"low"/"high"，CLI -effort 可调，默认 low）。
var simEffort = "low"

// SetSimEffort 设置 reasoning effort 档位（CLI/API 入口）。
func SetSimEffort(effort string) {
	simEffort = effort
}

// phaseThinkCharsLow 各门禁阶段 assistant 消息 thinking 基数（字符，reasoning low 口径）。
// 基线：统计自真实 DB messages.thinking_content（204 条，均值 472、范围 6-5629，高度右偏）；
// 阶段基数按 2026-08-08 真机窗口实测校准（mimo-v2.5 reasoning low 分阶段均值）。
var phaseThinkCharsLow = map[string]int{
	"init":     250,
	"prepare":  370,
	"outline":  437,
	"write":    145,
	"review":   701,
	"maintain": 164,
}

// phaseThinkCharsHigh 各门禁阶段 thinking 基数（reasoning high 口径）。
// 2026-08-16 真机批量会话（mimo-v2.5 reasoning_effort=high）反推：主会话 read 请求
// comp 2.1-2.5K/次（参数仅 ~20 token，其余为思考），review 阶段读正文思考 ~2.2K →
// review 基数 2100；其余阶段按 low 同比例（×3）放大。工具参数输出（大纲/审稿报告）
// 由 plays 内容本身建模，不在此表内。
var phaseThinkCharsHigh = map[string]int{
	"init":     750,
	"prepare":  1110,
	"outline":  1311,
	"write":    435,
	"review":   2100,
	"maintain": 492,
}

// simThinkRNG thinking 采样随机源（固定 seed 42，可复现）。
var simThinkRNG = rand.New(rand.NewSource(42))

// thinkingForPhase 返回某阶段的推理长度（字符），未知阶段回退 write。
// 采样真实分布（2026-08-12 从 DB 提取 204 条：10 等分位
// 6/22/44/79/122/177/247/371/595/965/3267/5629，右偏——多数请求短思考、
// 少数超长）：以阶段基数为中位锚点，按对数分布采样（固定 seed 可复现），
// 替代固定均值——固定均值高估了多数请求的 thinking（占 miss 15%）。
func thinkingForPhase(phase string) int {
	table := phaseThinkCharsLow
	if simEffort == "high" {
		table = phaseThinkCharsHigh
	}
	base, ok := table[phase]
	if !ok {
		base = table["write"]
	}
	if base <= 0 {
		return 0
	}
	// 对数均匀采样：多数落在 base/3 ~ base*1.5，约 5% 落在 base*2~base*5（长思考尾部）
	u := simThinkRNG.Float64()
	switch {
	case u < 0.15:
		return int(float64(base) * (0.2 + 0.3*simThinkRNG.Float64()))
	case u < 0.55:
		return int(float64(base) * (0.5 + 0.5*simThinkRNG.Float64()))
	case u < 0.9:
		return int(float64(base) * (1.0 + 0.5*simThinkRNG.Float64()))
	case u < 0.97:
		return int(float64(base) * (1.8 + 1.2*simThinkRNG.Float64()))
	default:
		return int(float64(base) * (3.0 + 4.0*simThinkRNG.Float64()))
	}
}

// thinkingText 生成 n 字符的推理占位文本（固定字符集循环，可复现；
// 中文密度与真实推理接近，tiktoken 精确计数后直接作为 token 参与估算）。
var thinkingRunes = []rune("审视当前任务目标与上下文约束权衡多种方案推演后果决定下一步行动选择最稳妥路径避免遗漏关键细节保持叙事连贯性确保设定一致推进情节发展")

func thinkingText(n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteRune(thinkingRunes[i%len(thinkingRunes)])
	}
	return b.String()
}

// ---- 固定前缀（与 writeSystemMessages 对应的三条 system，内容取自真实生成器） ----

var (
	identityText string // agentcfg.AgentIdentity(MainAgent)（真实）
	alwaysText   string // agentcfg.BuildAlwaysSkillsContent（真实，扫描 mode: always）
	catalogText  string // agentcfg.BuildSkillCatalog（真实，扫描 mode: auto）
	subSkillsText string // sub- 前缀技能拼接（review 子代理注入用，与 agent.buildSubagentSkills 同源）
)

// loadSystemTexts 用真实生成器构造固定前缀：
// identity/always/catalog 与 app/chat.go writeSystemMessages 完全一致，
// subSkills 与 internal/agent buildSubagentSkills 一致（扫描 sub- 前缀）。
// 这样模拟与真实请求同源，技能清单变动（新增/合并 skill）自动同步，零硬编码。
func loadSystemTexts() {
	store, err := skill.NewStore(slog.Default(), "")
	if err != nil {
		store = nil
	}
	simStore = store // 全局技能存储：realToolResult 的 ToolContext.SkillStore 与 skillContent 三层查找共用
	var meta []skill.SkillMeta
	if store != nil {
		meta = store.ListMeta(0)
		identityText = agentcfg.AgentIdentity(agentcfg.MainAgent)
		alwaysText = agentcfg.BuildAlwaysSkillsContent(meta, store, 0)
		catalogText = agentcfg.BuildSkillCatalog(store.ListMetaForCatalog(meta))
		subSkillsText = buildSubSkills(meta, store)
	} else {
		identityText = `你是 goink 小说创作系统的主创作助手。`
		alwaysText = ""
		catalogText = ""
		subSkillsText = ""
	}
}

// buildSubSkills 扫描 sub- 前缀技能并拼接内容（与 internal/agent buildSubagentSkills 同逻辑）。
func buildSubSkills(meta []skill.SkillMeta, store *skill.Store) string {
	var b strings.Builder
	for _, m := range meta {
		if !strings.HasPrefix(m.Name, "sub-") {
			continue
		}
		sk, ok := store.Get(0, m.Name)
		if !ok {
			continue
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.RawContent)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

// repoRoot 返回项目根目录（库包在 internal/cacheprobe/sim.go，向上三级；cmd 时代向上两级）。
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// file = <root>/internal/cacheprobe/sim.go → 向上三级到根
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// readFileText 读文件正文（去掉 YAML frontmatter），失败返回 fallback。
// 路径相对仓库根解析，保证 go run 与 go test 行为一致。
func readFileText(path string, max int) string {
	b, err := os.ReadFile(filepath.Join(repoRoot(), path))
	if err != nil {
		return "（读文件失败: " + err.Error() + "）"
	}
	s := string(b)
	if idx := strings.Index(s, "---\n"); idx >= 0 {
		s = s[idx+4:]
		if idx2 := strings.Index(s, "---\n"); idx2 >= 0 {
			s = s[idx2+4:]
		}
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// readFilesText 拼接多个内置 skill 的内容（read_required 的返回）。
func readFilesText(names []string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString("--- ")
		b.WriteString(n)
		b.WriteString(" ---\n")
		b.WriteString(skillContent(n))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func fixedSystem() []map[string]any {
	// 与真实 writeSystemMessages 一致：空串跳过（identity/always/catalog 各自独立消息）
	var msgs []map[string]any
	for _, c := range []string{identityText, alwaysText, catalogText} {
		if c != "" {
			msgs = append(msgs, sysMsg(c))
		}
	}
	return msgs
}

// fixedSystemNoCat 无 catalog 的固定前缀（auto-inject 后 catalog 可以去掉，技能已被系统注入）。
func fixedSystemNoCat() []map[string]any {
	var msgs []map[string]any
	for _, c := range []string{identityText, alwaysText} {
		if c != "" {
			msgs = append(msgs, sysMsg(c))
		}
	}
	return msgs
}

// novelState 模拟 NS，与真实 agentcfg.NovelState 字节格式完全一致：
//   【小说基础信息】书名/类型/简介 ← 真实 DB（novels 表第一本，经 GOINK_DATA_DIR/GOINK_DB_PATH）
//   当前进度：第 N 章 ← turn 动态（模拟创作推进；真实为 DB chapters 计数）
//   【章节指纹（最近）】← 真实 goink.md 尾部 1500 字符（platform.DataDir()/novels/{id}/goink.md）
// DB/文件缺失时回退占位，保证相对比较不受影响。
func novelState(turn int) string {
	var b strings.Builder
	b.WriteString(agentcfg.NovelStatePrefix)
	title, genre, desc := readRealNovelMeta()
	if title == "" {
		title, genre, desc = "焚天志", "东方玄幻", "少年秦烈身怀异火，踏入万界，快意恩仇。"
	}
	fmt.Fprintf(&b, "书名：%s\n", title)
	if genre != "" {
		fmt.Fprintf(&b, "类型：%s\n", genre)
	}
	if desc != "" {
		fmt.Fprintf(&b, "简介：%s\n", desc)
	}
	// 进度锚点：真实 DB chapters 计数 + 模拟 turn 偏移（模拟窗口从真实进度续写，
	// 对齐真机 agentcfg.NovelState 的 DB chapters 计数口径）
	realCh := readRealChapterCount()
	prog := turn
	if realCh > prog {
		prog = realCh
	}
	fmt.Fprintf(&b, "当前进度：第 %d 章。创作须服务于全书总纲（book-outline.md），只展开本卷情节，后续卷设定不得提前使用。\n", prog)

	// 真实 goink.md 指纹（若存在）
	real := readRealGoinkFingerprint()
	if real != "" {
		b.WriteString("\n【章节指纹（最近）】\n")
		r := []rune(real)
		if len(r) > 1500 {
			b.WriteString(string(r[len(r)-1500:]))
			b.WriteString("\n…（更早指纹已截断，如需完整内容用 read(goink.md)）\n")
		} else {
			b.WriteString(real)
			if !strings.HasSuffix(real, "\n") {
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	// 回退占位指纹（每章 6 段指纹，约 120 字符/章，模拟 1500 字符上限）
	b.WriteString("\n【章节指纹（最近）】\n")
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "### 第%d章 %s\n\n开篇：%s\n\n场景：%s\n\n情感：%s\n\n对白：%s\n\n钩子：%s\n\n感官：%s\n\n",
			i, "秘境初探", "动作开场", "宗门演武", "紧张", "冲突对话", "悬念", "视觉听觉")
	}
	return b.String()
}

// readRealNovelMeta 从真实 DB 读取第一本小说的 书名/类型/简介（与 agentcfg.NovelState 同源）。
// DB 路径：GOINK_DB_PATH > GOINK_DATA_DIR/novel-agent.db > exe 目录 > ~/Goink。
func readRealNovelMeta() (title, genre, desc string) {
	db, err := openRealDB()
	if err != nil {
		return "", "", ""
	}
	defer db.DB()

	var n novel.Novel
	if err := db.First(&n).Error; err != nil {
		return "", "", ""
	}
	return n.Title, n.Genre, n.Description
}

// readRealChapterCount 读取真实 DB 第一本小说的章节数（NS"当前进度"锚点用，
// 对齐真机 agentcfg.NovelState 的 DB chapters 计数）。DB 缺失时返回 0（调用方回退 turn）。
func readRealChapterCount() int {
	id := realNovel()
	if id == 0 {
		return 0
	}
	db, err := openRealDB()
	if err != nil {
		return 0
	}
	defer db.DB()
	var cnt int64
	if err := db.Model(&chapter.Chapter{}).Where("novel_id = ?", id).Count(&cnt).Error; err != nil {
		return 0
	}
	return int(cnt)
}

// openRealDB 打开真实 DB（只读）。gorm sqlite driver。
var realDBOnce sync.Once
var realDB *gorm.DB

func openRealDB() (*gorm.DB, error) {
	realDBOnce.Do(func() {
		path := os.Getenv("GOINK_DB_PATH")
		if path == "" {
			path = filepath.Join(platform.DataDir(), "novel-agent.db")
		}
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		})
		if err != nil {
			return // 打开失败：realDB 保持 nil，调用方 fallback 默认值
		}
		realDB = db
	})
	if realDB == nil {
		return nil, fmt.Errorf("open real db failed")
	}
	return realDB, nil
}

// readRealGoinkFingerprint 读取真实 goink.md 尾部最近 1500 字符（与 agentcfg.NovelState 的 maxGoinkChars 一致）。
// 查找路径：GOINK_DATA_DIR 或 exe 目录或 ~/Goink 下的 novels/{1..N}/goink.md（取存在的一本）。
func readRealGoinkFingerprint() string {
	dir := platform.DataDir()
	novelsDir := filepath.Join(dir, "novels")
	entries, err := os.ReadDir(novelsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(novelsDir, e.Name(), "goink.md")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return string(b)
	}
	return ""
}

// ---- 真实工具执行（用户要求：模拟真机数据行为）----

// realNovelIDOnce 真实 DB 第一本小说的 ID（工具执行的 NovelID）。
var (
	realNovelOnce sync.Once
	realNovelID   int64
)

func realNovel() int64 {
	realNovelOnce.Do(func() {
		db, err := openRealDB()
		if err != nil {
			return
		}
		var n novel.Novel
		if err := db.First(&n).Error; err == nil {
			realNovelID = n.ID
		}
	})
	return realNovelID
}

// realToolResult 生成工具结果消息内容：
// 只读工具（get_*/search_*/read）走真实 Registry.Execute 读真实 DB 副本，
// 返回与真机 resultJSON 等价的 content（{"success":true,"data":{...}}）；
// 写工具（edit/update/create/delete/run_subagent/set_phase/auto_skill_injection）
// 按工具类型生成与真机同结构的模板结果（不污染副本、保持可复现）——
// 真机返回结构：create/update 类 {"id":N} 或 {"ids":[...],"count":N}，edit {"path","change_type","approved"}。
func realToolResult(name, args, fallback string) string {
	if isReadonlyTool(name) {
		if out, ok := execRealTool(name, args); ok {
			return out
		}
	}
	return writeToolResult(name, args, fallback)
}

// writeToolResult 写工具结果模板：对齐真机各写工具返回结构（从真实 DB messages 表
// role=tool 消息提取校准，2026-08-12）——工具结果占 miss 构成 ~38%，结构一致才能
// 让成本口径贴近真机。统一格式 {"success":true,"data":{...}}。
func writeToolResult(name, args, fallback string) string {
	ok := func(data map[string]any) string {
		b, _ := json.Marshal(map[string]any{"success": true, "data": data})
		return string(b)
	}
	switch {
	case name == "edit":
		// 真机 rw_tools.go:237-249：{"approved","change_type","path"}；append 为主（指纹账本）
		path := ""
		changeType := "full_replace"
		var a struct {
			Path       string `json:"path"`
			ChangeType string `json:"change_type"`
		}
		if json.Unmarshal([]byte(args), &a) == nil {
			path = a.Path
			if a.ChangeType != "" {
				changeType = a.ChangeType
			}
		}
		return ok(map[string]any{"approved": true, "change_type": changeType, "path": path})
	case name == "set_phase":
		// 真机 agent.go:583：{"phase":X}（成功）或 {"current_phase":X,"error":...}（失败）
		phase := phaseOfArgs(args)
		if phase == "" {
			return wrapResult(fallback)
		}
		return ok(map[string]any{"phase": phase})
	case name == "update_chapter_meta" || name == "update_writing_snapshot":
		// 真机：{"updated":true}
		return ok(map[string]any{"updated": true})
	case name == "update_chapter_plan":
		// 真机：{"scope":"near"/"next"}
		scope := "near"
		var a struct {
			Scope string `json:"scope"`
		}
		if json.Unmarshal([]byte(args), &a) == nil && a.Scope != "" {
			scope = a.Scope
		}
		return ok(map[string]any{"scope": scope})
	case name == "update_character_relationship":
		// 真机：{"action":"evolve","id":N}
		return ok(map[string]any{"action": "evolve", "id": simNextID()})
	case name == "update_reader_perspective_entry":
		// 真机：{"id":N,"revealed_chapter":N}
		return ok(map[string]any{"id": simNextID(), "revealed_chapter": simNextID()})
	case name == "create_timeline_entry":
		// 真机：{"count":1,"ids":[N]}
		return ok(map[string]any{"count": 1, "ids": []int64{simNextID()}})
	case name == "delete_record":
		// 真机：{"deleted":{"id":N,"name":"...","type":"..."}}
		return ok(map[string]any{"deleted": map[string]any{"id": simNextID(), "name": "删除对象", "type": "character"}})
	case strings.HasPrefix(name, "create_"):
		// 真机 create 类（scene/item_occurrence 等）：{"id":N}
		return ok(map[string]any{"id": simNextID()})
	case strings.HasPrefix(name, "update_"):
		// 真机 update 类（character/timeline_entry/arc_node 等）：{"id":N}
		return ok(map[string]any{"id": simNextID()})
	case name == "run_subagent":
		// 真机子代理返回审读报告（playResult 已有完整 report fallback）
		return wrapResult(fallback)
	default:
		return wrapResult(fallback)
	}
}

// simNextID 单调递增的模拟 ID（写工具模板结果用，可复现）。
var simIDCounter int64

func simNextID() int64 {
	simIDCounter++
	return simIDCounter
}

// isReadonlyTool 判定只读工具（无副作用，可安全真实执行）。
func isReadonlyTool(name string) bool {
	if name == "read" {
		return true
	}
	return strings.HasPrefix(name, "get_") || strings.HasPrefix(name, "search_")
}

// execRealTool 用真实工具注册表执行只读工具（读真实 DB）。
func execRealTool(name, args string) (string, bool) {
	id := realNovel()
	if id == 0 || simRegistry == nil {
		return "", false
	}
	db, err := openRealDB()
	if err != nil {
		return "", false
	}
	res := simRegistry.Execute(context.Background(), name, json.RawMessage(args), mcp_tools.ToolContext{
		DB:         db,
		NovelID:    id,
		SkillStore: simStore,
	}, nil)
	if res == nil || !res.Success {
		return "", false
	}
	return resultJSON(res), true
}

// resultJSON 对齐真实 agent 工具结果格式（agent/safety.go toolOutput.resultJSON）。
func resultJSON(res *mcp_tools.ToolResult) string {
	payload := map[string]any{}
	if res != nil {
		payload["success"] = res.Success
		if res.Error != "" {
			payload["error"] = res.Error
		}
		if res.Data != nil {
			payload["data"] = res.Data
		}
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// wrapResult 把 fallback 内容包装成真机 resultJSON 格式（写工具/真实执行失败时用）。
func wrapResult(fallback string) string {
	var data any
	if json.Unmarshal([]byte(fallback), &data) == nil {
		payload := map[string]any{"success": true, "data": data}
		b, _ := json.Marshal(payload)
		return string(b)
	}
	return fmt.Sprintf(`{"success":true,"data":%q}`, fallback)
}

// chapterNumRe 匹配 edit 参数中的章节号（chapters/025.md）。
var chapterNumRe = regexp.MustCompile(`chapters/(\d+)\.md`)

// playResult 生成 play 的工具结果内容（真实执行优先，fallback 包装兜底）。
func playResult(p play) string {
	return realToolResult(p.tool, p.args, p.result)
}

// phaseOfArgs 从 set_phase 参数 JSON 中提取纯阶段名（{"phase":"write"} → "write"）。
func phaseOfArgs(args string) string {
	var a struct {
		Phase string `json:"phase"`
	}
	if json.Unmarshal([]byte(args), &a) == nil && a.Phase != "" {
		return a.Phase
	}
	return args
}

// simGateBlockRate 门禁拦截建模概率（0=关闭；真机实测 set_phase 失败率 25%——
// require 未满足/技能未读/字数未校验时拦截，失败消息全 miss 且带动重试）。
// 包级开关（单线程模拟器），RunWindowMode/CLI 可配置；固定 seed 可复现。
var simGateBlockRate float64

// simBlockRNG 拦截建模随机源（固定 seed 42，可复现）。
var simBlockRNG = rand.New(rand.NewSource(42))

// blockGateTransition 按概率模拟 set_phase 被门禁拦截：
// 失败时注入真实格式 reminder（{"success":false,"error":...,"current_phase":...}，
// 对齐 agent.go:586-588），不切换阶段，返回 false（调用方保持原阶段）。
func blockGateTransition(cur []map[string]any, target string) ([]map[string]any, bool) {
	if simGateBlockRate <= 0 || simBlockRNG.Float64() >= simGateBlockRate {
		return cur, true
	}
	errMsg := "阶段 [" + simPhase + "] 要求必须调用以下工具后才能切换到 [" + target + "]，当前未调用: [get_writing_snapshot]"
	reminder := fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>", fmt.Sprintf(`{"success":false,"error":%q,"current_phase":%q}`, errMsg, simPhase))
	cur = append(cur, userMsg(reminder))
	return cur, false
}
// tool_calls + 1 次 thinking，与真机 LLM 并行行为一致），set_phase 单独成组并更新 simPhase。
// 组上限 10（真机单次最多 ~9 个并行调用）。
// 回调：onSubagent 在 run_subagent play 后插入子代理请求序列；onRead 记录 read 调用 ID；
// onPhase 在阶段切换时触发（返回新 cur，用于 auto 模式技能注入/read 清理等）。
func runPlays(cache *TokenCache, history, cur []map[string]any, plays []play, results *[][2]int64,
	onSubagent func(cur []map[string]any) [][2]int64, onRead func(id string), onPhase func(cur []map[string]any, phase string) []map[string]any) []map[string]any {
	const maxGroup = 10
	for i := 0; i < len(plays); {
		j := i
		for j < len(plays) && j-i < maxGroup && plays[j].tool != "set_phase" && plays[j].tool != "run_subagent" {
			j++
		}
		if j == i {
			j = i + 1 // set_phase / run_subagent 单独一组
		}
		group := plays[i:j]

		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		*results = append(*results, [2]int64{hit, miss})

		ids := make([]string, len(group))
		names := make([]string, len(group))
		argsList := make([]string, len(group))
		for k, p := range group {
			ids[k] = fmt.Sprintf("call_p%d_%d", i, k)
			names[k] = p.tool
			argsList[k] = p.args
			// 章节进度推进（打点记录用）：edit chapters/NNN.md 即进入第 N 章。
			// 取最大章：批量批末统一审稿 reviewPlays(1) 会回改 chapters/001.md，
			// 若直接覆盖会把进度拽回 1（刻度反序 bug：256K 显示第 1 章 < 128K 第 3 章）。
			if p.tool == "edit" {
				if m := chapterNumRe.FindStringSubmatch(p.args); m != nil {
					var n int
					fmt.Sscanf(m[1], "%d", &n)
					if n > simCurrentChapter {
						simCurrentChapter = n
					}
				}
			}
		}
		cur = append(cur, asstToolCalls(ids, names, argsList))
		for k, p := range group {
			switch p.tool {
			case "set_phase":
				// 解析 {"phase":"xxx"} 得到纯阶段名：simPhase（thinking 长度）与 onPhase
				// 技能注入（phaseInjectSkills key 是纯阶段名）都依赖它——旧代码直接传
				// p.args JSON，导致 injectPhaseOn 永远查不到 key，阶段技能注入从未生效。
				// 同步真实 agent.go：只真切换（from != to）注入技能，同阶段 set_phase
				// （批量 write 章边界）跳过——技能已在上下文，重复注入纯浪费。
				phase := phaseOfArgs(p.args)
				// 门禁拦截建模：真机 set_phase 25% 失败（require 未满足/技能未读/字数未校验），
				// 失败注入 reminder 且不切换阶段；LLM 下轮重试（重试请求自然计入 miss）。
				var ok bool
				cur, ok = blockGateTransition(cur, phase)
				if !ok {
					cur = append(cur, toolMsg(ids[k], p.tool, fmt.Sprintf(`{"success":false,"error":"require 未满足","current_phase":%q}`, simPhase)))
					continue
				}
				if phase != simPhase && onPhase != nil {
					cur = onPhase(cur, phase)
				}
				simPhase = phase
				cur = appendPhase(cur, simPhase, true)
			case "read", "read_required", "auto_skill_injection":
				if onRead != nil {
					onRead(ids[k])
				}
			}
			if p.tool == "run_subagent" && onSubagent != nil {
				subResults := onSubagent(cur)
				*results = append(*results, subResults...)
			}
			cur = append(cur, toolMsg(ids[k], p.tool, playResult(p)))
		}
		i = j
	}
	return cur
}

// filterReadRequired auto 模式下过滤技能读取 play（技能已在 set_phase 时系统注入）。
// 兼容两代工具名：read_required（旧）与 auto_skill_injection（8/9 改名，技能全文）。
func filterReadRequired(plays []play, mode string) []play {
	if mode != "auto" {
		return plays
	}
	out := make([]play, 0, len(plays))
	for _, p := range plays {
		if p.tool == "auto_skill_injection" || p.tool == "read_required" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// skillDedupSim 包级开关（单线程模拟器）：true 时阶段技能按"短提醒"策略注入——
// 首次进入阶段注入技能全文（学习内容，进入历史常驻），后续再进入同阶段时
// 只注入"当前阶段应遵循技能"的短提醒（技能名清单，~百 token，紧跟本轮请求尾部，
// 注意力最强位置），不再重复注入全文。解决"全文在历史里但模型没注意"的
// Lost in the Middle 问题：全文保证可见，短提醒保证被注意。
var skillDedupSim bool

// phaseSkillReminder 各阶段技能短提醒（技能名 + 一句要点），注入成本 ~百 token。
func phaseSkillReminder(phase string) string {
	names := map[string][]string{
		"prepare":  {"common-sense-logic（现实逻辑一致性）"},
		"outline":  {"chapter-hook-enhanced（章末钩子）、chapter-title-design（章名设计）"},
		"write":    {"show-dont-tell（画面化）、anti-ai-writing（去 AI 味）、pov-purity（视角纯净）、info-density（信息密度）"},
		"review":   {"revision-pass（自审修改）"},
		"maintain": {"anti-repetition（防重复）、foreshadow-cycle（伏笔循环）"},
	}
	ns, ok := names[phase]
	if !ok {
		return ""
	}
	s := "【当前阶段技能提醒】"
	for i, n := range ns {
		if i > 0 {
			s += "；"
		}
		s += n
	}
	return s + "。请按以上技能要点创作，勿偏离。"
}

// visibleIn 判定一段全文是否已在模型视野中（完整消息序列的 system 消息里能找到相同 content）。
// 这是"AI 能不能看到 skill"的唯一判定标准：模型每轮收到的 messages 数组里，
// 存在 role=system 且 content 与该技能全文完全相同的消息。
func visibleIn(msgs []map[string]any, content string) bool {
	for _, m := range msgs {
		if role, _ := m["role"].(string); role == "system" {
			if c, ok := m["content"].(string); ok && c == content {
				return true
			}
		}
	}
	return false
}

// injectPhaseOn auto 模式阶段切换时注入阶段技能（返回新 cur）。
// cur 是当轮新增消息（未落库），history 是已落库历史——模型完整视野 = history + cur。
// 去重开启时：
//   完整视野中无全文 → 注入全文（首次进入阶段，学习内容）；
//   已有全文 → 只注入短提醒（技能名清单，唤起注意，不再重复全文）。
func injectPhaseOn(mode string, history, cur []map[string]any, phase string) []map[string]any {
	if mode == "auto" {
		if sk, ok := phaseInjectSkills[phase]; ok && sk != "" {
			if skillDedupSim && (visibleIn(history, sk) || visibleIn(cur, sk)) {
				// 全文已在视野：注入短提醒（首次提醒也注入；全文+提醒都常驻历史）
				if rm := phaseSkillReminder(phase); rm != "" {
					cur = append(cur, sysMsg(rm))
				}
				return cur
			}
			cur = append(cur, sysMsg(sk))
			if skillDedupSim {
				if rm := phaseSkillReminder(phase); rm != "" {
					cur = append(cur, sysMsg(rm))
				}
			}
		}
	}
	return cur
}

// ---- 短问答场景 ----

// 每轮消息构造策略（两种协议的核心差异）：
//   now（修复后）：NS 随 user 落库 → 进入历史；请求 = 历史（含全部 NS）+ 新 user + 新 NS
//   legacy（修复前）：NS 不落库 → 历史无 NS；当轮请求 = 历史 + user + 当轮 NS（紧跟 user）
// 两种模式下 NS 都紧跟当轮 user 消息（旧实现把 loadAPIMessages 结果 append 后交给
// agent 循环，NS 在 user 之后、工具循环之前，且当轮内保持）。唯一差异是历史是否保留 NS。


func shortAnswer() string {
	return "此界修炼体系：引气 → 筑基 → 金丹 → 元婴 → 化神。主角以异火之拥独特路径修行，宗门以灵石为货币。"
}

// ---- 门禁创作场景 ----

// play 一次工具调用（assistant tool_call + tool 结果）
type play struct {
	tool   string
	args   string
	result string
}

// 章节正文（固定种子，可复现）：每章正文独立生成，目标字数按真实章节字数分布波动
// （均值 = 设置的 (min+max)/2，std 取真实章节实测 386 字符），拆成 6 段逐次 edit 追加
// ——对应 write 阶段真实写作量级，且章与章之间长度/内容不同。
var chapterBodies [][]string

// simChapterTarget 每章目标字数（波动后），与 chapterBodies 同索引。
var simChapterTarget []int

// maxSimChapters 预生成章节数（覆盖单章轮 + 批量 + 子代理读取的最大章号；
// 批量 256 章 ≈ 1M 历史，覆盖 1024K 刻度）。
const maxSimChapters = 256

// realWordStdDev 真实章节字数标准差（字符，D:\Goink\novels 19 章实测：
// 均值 3319、std 386、范围 2652-4073，设置范围 2500-4000）。
const realWordStdDev = 386.0

// 章节字数配置（从真实 DB app_config 读取，模拟失败回退默认 2500-4000）。
var (
	simMinWords int
	simMaxWords int
	simMeanWords int
)

// readRealWordRange 从真实 DB 读章节字数上下限（get_chapter_list 校验用）。
// DB 路径与 readRealNovelMeta 相同（GOINK_DB_PATH > GOINK_DATA_DIR > exe 目录 > ~/Goink）。
func readRealWordRange() (int, int) {
	db, err := openRealDB()
	if err != nil {
		return 2500, 4000
	}
	var s config.AppSettings
	if err := db.First(&s, 1).Error; err != nil {
		return 2500, 4000
	}
	minW, maxW := s.MinChapterWords, s.MaxChapterWords
	if minW < 100 || maxW <= minW {
		return 2500, 4000
	}
	return minW, maxW
}

func initBody() {
	simMinWords, simMaxWords = readRealWordRange()
	simMeanWords = simMinWords + (simMaxWords-simMinWords)/2
	mean := simMeanWords

	rng := rand.New(rand.NewSource(42))
	const chars = "天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏金木水火土风雨雷电山海林木龙虎凤凰" +
		"云霞烟雾雪霜星辰日月阴阳乾坤八卦五行太极剑刀枪戟弓矢戈矛阵法宗派师徒心法内力武技神通" +
		"灵气灵根丹田金丹元婴化神散仙真仙金仙大罗斩妖除魔御剑飞行炼丹炼器符箓阵旗禁制秘境传承" +
		"试炼机缘造化天骄妖孽至尊大帝圣主神王主宰荒古太古远古上古中古近古百年千年万年永恒不朽" +
		"苍茫大地九霄之上万界诸天三千世界浩瀚星空无垠宇宙轮回因果宿命机缘命数气运天道法则"
	runes := []rune(chars)

	chapterBodies = make([][]string, maxSimChapters)
	simChapterTarget = make([]int, maxSimChapters)
	for ch := 0; ch < maxSimChapters; ch++ {
		// 目标字数：均值 + 正态波动（真实 std），clamp 到设置范围
		target := int(rng.NormFloat64()*realWordStdDev + float64(mean))
		if target < simMinWords {
			target = simMinWords
		}
		if target > simMaxWords {
			target = simMaxWords
		}
		simChapterTarget[ch] = target
		segLen := target / 6
		var b strings.Builder
		for i := 0; i < target; i++ {
			b.WriteRune(runes[rng.Intn(len(runes))])
		}
		body := b.String()
		segs := make([]string, 6)
		for i := 0; i < 6; i++ {
			start := i * segLen
			end := (i + 1) * segLen
			if end > len(body) {
				end = len(body)
			}
			segs[i] = body[start:end]
		}
		chapterBodies[ch] = segs
	}
}

// editArgs 构造 edit 工具的 arguments JSON（含真实长度的 content）。
func editArgs(path, content string) string {
	b, err := json.Marshal(map[string]any{
		"path":        path,
		"change_type": "append",
		"content":     content,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// readSkill 构造一次 read 调用（读取 skill 文件，返回真实内容量级）。
func readSkill(name string) play {
	return play{
		tool:   "read",
		args:   fmt.Sprintf(`{"path":"skills/%s.md"}`, name),
		result: skillContent(name),
	}
}

// readRequired 模拟门禁 auto_skill_injection 工具调用（2026-08-08 起各阶段必读技能用此加载，
// 工具名与真实业务一致：auto_skill_injection，args 为逗号分隔技能名列表）。
func readRequired(names ...string) play {
	skills := strings.Join(names, ",")
	return play{
		tool:   "auto_skill_injection",
		args:   fmt.Sprintf(`{"skills":"%s"}`, skills),
		result: readFilesText(names),
	}
}

// ── auto-inject 方案（对照研究）：把各阶段必读技能以 system 消息在阶段开头注入，
// 不再依赖 read_required 工具调用。与 read_required 对比验证缓存差异。 ──

// initInject init 阶段必读技能（5 个开书技能），auto 模式在 init 开始时注入。
var initInject = readFilesText([]string{"main-core-init-phase", "main-tech-genre-templates", "main-tech-book-outline", "main-tech-character-design", "main-tech-world-building-system"})

// phaseInjectSkills 各阶段必读技能 → 注入内容（与门禁 auto_skill_injection 一致）。
var phaseInjectSkills = map[string]string{
	"prepare":  readFilesText([]string{"main-tech-common-sense-logic"}),
	"outline":  readFilesText([]string{"main-tech-chapter-hook-enhanced", "main-tech-chapter-title-design"}),
	"write":    readFilesText([]string{"main-tech-show-dont-tell", "main-tech-anti-ai-writing", "main-tech-pov-purity", "main-tech-info-density"}),
	"maintain": readFilesText([]string{"main-tech-anti-repetition", "main-tech-foreshadow-cycle"}),
}

// injectSkillsPlays 把 plays 里的 read_required 移除，改为在进入新阶段时注入对应技能 system 消息。
// 返回 (plays, 阶段注入点)。阶段注入点:sets 元素 = 阶段名，进入该阶段时需注入 phaseInjectSkills[phase]。
// 实现：遍历 plays，遇到 set_phase 记录目标阶段；read_required 直接跳过（改为注入）。
func injectSkillsPlays(plays []play) ([]play, []string) {
	var out []play
	var injects []string
	seen := map[string]bool{}
	for _, p := range plays {
		if p.tool == "auto_skill_injection" || p.tool == "read_required" {
			continue // 技能改为 system 注入，不再工具调用
		}
		if p.tool == "set_phase" && !seen[p.args] {
			if _, ok := phaseInjectSkills[p.args]; ok {
				injects = append(injects, p.args)
				seen[p.args] = true
			}
		}
		out = append(out, p)
	}
	return out, injects
}

// initScript 开书（init）流程：read_required 加载必读技能（门禁配置 auto_skill_injection 驱动）
// + 建世界观/角色/弧线 + 写总纲 + 建卷
func initScript() []play {
	return []play{
		readRequired(skillsFor("single", "init")...),
		{tool: "create_location", args: `{"name":"青云宗","type":"门派","desc":"主角所在宗门"}`, result: `{"id":1}`},
		{tool: "create_character", args: `{"name":"陆沉","desc":"主角","location_id":1}`, result: `{"id":1}`},
		{tool: "create_character", args: `{"name":"柳雪","desc":"师姐","location_id":1}`, result: `{"id":2}`},
		{tool: "create_lore", args: `{"title":"天地灵气","category":"规则","content":"灵气浓度决定修炼速度"}`, result: `{"id":1}`},
		{tool: "create_item", args: `{"name":"聚气丹","owner_id":1,"narrative_role":"道具"}`, result: `{"id":3}`},
		{tool: "create_timeline_entry", args: `{"title":"暗流涌动","category":"foreshadowing","target_chapter":8,"importance":"high"}`, result: `{"id":5}`},
		{tool: "create_preference", args: `{"category":"style","content":"文风简练、细节丰富"}`, result: `{"id":1}`},
		{tool: "create_story_arc", args: `{"name":"第一卷·崛起","arc_type":"volume","start_chapter":1,"end_chapter":20,"description":"主角入宗崛起"}`, result: `{"id":1}`},
		{tool: "edit", args: editArgs("book-outline.md", "# 全书总纲\n核心矛盾：主角从废柴逆袭对抗宗门暗流\n主角成长弧线：入宗→觉醒→崛起\n结局方向：登顶青云\n篇幅规划：3 卷 60 章"), result: "写入 book-outline.md"},
		{tool: "edit", args: editArgs("book-outline.md", "## 第一卷规划\n第 1-20 章：入宗与崛起，暗流初现\n爽点分布：每章至少 1 个\n伏笔主线：暗流涌动（第 8 章回收）"), result: "补充 book-outline.md 卷规划"},
		{tool: "set_phase", args: `{"phase":"prepare"}`, result: `{"success":true,"phase":"prepare"}`},
	}
}

// gateScript 一轮创作的完整工具剧本，严格对照 main-core-writing-kernel 阶段指令：
// prepare（9 required 查询 + lore/items + 必读 1 + 按需 2）→ outline（必读 2 + 类型 1 + 2 次大纲 edit）
// → write（必读 4 + 读大纲 + 6 段正文 + 字数校验重写 + 物品记录）→ 自审（2 技能 + 1 次修改）
// → review（run_subagent + 自查重读 + 3 处修复 + 复查）→ maintain（先读 2 技能 + 7 查询 + 2 搜索
// + 11 项更新 + goink.md 指纹）
func gateScript(turn int) []play {
	ch := turn + 1
	var plays []play
	plays = append(plays, preparePlays(ch)...)
	plays = append(plays, outlinePlays(ch)...)
	plays = append(plays, writePlays(ch)...)
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`})
	plays = append(plays, selfReviewPlays(ch)...)
	plays = append(plays, reviewPlays(ch)...)
	plays = append(plays, maintainPlays(ch, "prepare")...)
	return plays
}

// preparePlays 阶段 prepare：9 项必查（门禁 require 强制）+ 加载 prepare 技能。
// 查询参数对齐 8/8 真机实测（窗口 1 写第 13 章，sess_1_18c9cd85fdd3d85c 18:25）：
// get_characters 精简 721 字符（brief 格式）、get_scenes 精简 2,545/47 场景（brief）、
// get_reader_perspective 全量 8,108、get_timeline/get_story_arcs current_chapter 窗口 2.7-2.9K、
// get_writing_context 全量 14.9K、get_chapter_list 4.2K。
func preparePlays(ch int) []play {
	return []play{
		{tool: "get_writing_context", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: longContext(ch)},
		{tool: "get_chapter_list", args: `{"size":5}`, result: chapterList(ch)},
		{tool: "get_characters", args: `{"brief":true}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","location_id":3},{"id":2,"name":"林雪","desc":"师姐","location_id":3}]}`},
		{tool: "get_timeline", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"arcs":[{"id":1,"name":"登天之路","type_zh":"主线","nodes_done":2,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "get_writing_snapshot", args: `{}`, result: fmt.Sprintf(`{"last_chapter_num":%d,"current_arc_id":1,"current_location":"青云宗","active_chars":["陈昊","林雪"]}`, ch-1)},
		{tool: "get_scenes", args: `{"brief":true}`, result: `{"scenes":[{"id":9,"title":"入门测验","summary":"陈昊通过测验","word_count":1200}]}`},
		{tool: "get_preferences", args: `{}`, result: `{"preferences":[{"category":"style","content":"快节奏、斗法细节"},{"category":"taboo","content":"禁止主角圣母"}]}`},
		readRequired(skillsFor("single", "prepare")...),
		readSkill("main-tech-genre-templates"),
		readSkill("main-tech-book-outline"),
		{tool: "set_phase", args: `{"phase":"outline"}`, result: `{"success":true,"phase":"outline"}`},
	}
}

// outlinePlays 阶段 outline：auto_skill_injection 必读（门禁配置驱动）+ 类型专精 1 个。
// 常备技能（book-outline/chapter-opening/maliang-method/dialogue-subtext/emotional-arc/emotion-injection）
// 首次会话加载一次，后续章节历史中已有则直接引用，不重复 read。
func outlinePlays(ch int) []play {
	return []play{
		readRequired(skillsFor("single", "outline")...),
		readSkill("main-type-xuanhuan-cultivation"),
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), outlineText(ch)), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), "## 关键事件\n1. 主角闯入秘境\n2. 遭遇袭击\n3. 突破瓶颈\n\n## 章末钩子\n屋外传来脚步声"), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`},
	}
}

// writePlays 阶段 write：auto_skill_injection 必读（门禁配置驱动）
// + read 本章大纲（kernel write 阶段第 2 步：read(required) 读 outlines/NNN.md，门禁 require 强制——
// 批量循环写多章时靠它锁定本章大纲，防止把别的章的大纲内容串进本章正文）+ 分段写正文。
// 情景技能（climax/shuangdian/foreshadow/emotion/pacing）仅本章涉及该情景时读，普通章不读；
// 字数规则由代码校验，不读 word-count-calibration。
// 不含阶段切换（由调用方决定何时转 review：single 每章转、batch 整批循环完才转）。
func writePlays(ch int) []play {
	plays := []play{
		readRequired(skillsFor("single", "write")...),
		play{tool: "read", args: fmt.Sprintf(`{"path":"outlines/%03d.md"}`, ch), result: outlineText(ch)},
	}
	plays = append(plays, writeBodyPlays(ch)...)
	return plays
}

// writeBodyPlays 正文写入 + 字数校验（单章/批量共用）：
// 6 段 edit 写满目标字数 → get_chapter_list 校验（首次欠字不达标）→ 补写 → 复查达标 → 物品记录。
func writeBodyPlays(ch int) []play {
	body := chapterBodies[ch-1]
	target := simChapterTarget[ch-1]
	plays := make([]play, 0, 11)
	for i := 0; i < 6; i++ {
		segLen := len([]rune(body[i]))
		written := (i + 1) * target / 6
		if written > target {
			written = target
		}
		plays = append(plays, play{
			tool:   "edit",
			args:   editArgs(fmt.Sprintf("chapters/%03d.md", ch), body[i]),
			result: fmt.Sprintf("写入 %d 字，当前 %d/%d", segLen, written, target),
		})
	}
	plays = append(plays,
		play{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(ch, simMinWords-100, false)},
		play{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "补写段落：主角凝视远方，回忆方才的惊险，掌心仍有余温。夜色中一道人影掠过屋檐，他握紧长剑，悄然跟了上去。"), result: fmt.Sprintf("补写 400 字，当前 %d/%d", target, target)},
		play{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(ch, target, true)},
		play{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"主角服用聚气丹"}`, result: "已记录"},
		// 写时把关（2026-08-16 门禁 require 新增）：write 转出前必须 check_story_consistency
		// 程序化核对四类硬错误（伏笔超期/角色断档/物品冲突/死者复出），current_chapter 必填
		play{tool: "check_story_consistency", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"ok":true,"issues":[]}`},
	)
	return plays
}

// chapterListCheck 构造 get_chapter_list 的字数校验返回。
func chapterListCheck(ch int, wordCount int, ok bool) string {
	return fmt.Sprintf(`{"check_chapter":%d,"word_count":%d,"word_count_ok":%v,"min_words":%d,"max_words":%d}`,
		ch, wordCount, ok, simMinWords, simMaxWords)
}

// selfReviewPlays 阶段 write 后自审：2 技能 + 1 次修改（kernel 阶段技能表 write后 行）。
func selfReviewPlays(ch int) []play {
	return []play{
		readSkill("main-tech-revision-pass"),
		readSkill("sub-tech-anti-ai-grade"),
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "自审修改：润色两处过渡，去除 AI 味用词。"), result: "自审完成"},
	}
}

// reviewPlays 阶段 review：run_subagent 审稿（require: run_subagent）+ 自查重读 + 3 处修复 + 字数复查。
func reviewPlays(ch int) []play {
	return []play{
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(ch)},
		// 审稿核对（真机 8/8 窗口 1 实测 18:30-18:32 序列：读正文核对 + 全套状态核对——
		// timeline/arcs/reader 与 prepare 相同参数重复调用，每次都是新消息 miss；
		// characters 用全量核对 status、items/locations 补查、check_story_consistency 自动核对）
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md","start_line":1,"end_line":100}`, ch), result: chapterBodies[ch-1][0] + chapterBodies[ch-1][1]},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md","start_line":100,"end_line":200}`, ch), result: chapterBodies[ch-1][2] + chapterBodies[ch-1][3]},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md","start_line":200,"end_line":300}`, ch), result: chapterBodies[ch-1][4] + chapterBodies[ch-1][5]},
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","status":"突破金丹"}]}`},
		{tool: "get_character_relations", args: `{}`, result: `{"relations":[{"a":"陈昊","b":"林雪","relation":"师姐弟"}]}`},
		{tool: "get_timeline", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "check_story_consistency", args: `{}`, result: `{"ok":true,"issues":[]}`},
		{tool: "get_items", args: `{"mode":"list","size":10}`, result: `{"items":[{"id":3,"name":"聚气丹"}]}`},
		{tool: "get_locations", args: `{"mode":"list","size":10}`, result: `{"locations":[{"id":5,"name":"青云宗"}]}`},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：调整对话节奏，补充情绪铺垫。"), result: "已修复问题 1"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：前文伏笔在此回收，强化悬念。"), result: "已修复问题 2"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：删减冗余描写，收紧节奏。"), result: "已修复问题 3"},
		{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(ch, simChapterTarget[ch-1], true)},
		{tool: "set_phase", args: `{"phase":"maintain"}`, result: `{"success":true,"phase":"maintain"}`},
	}
}

// maintainPlays 阶段 maintain：7 项状态查询 + 搜索防遗忘 + 6 类更新 + goink.md（require 13 项）。
// nextPhase 是阶段切换目标：single/batch 均回 "prepare"（batch done 阶段已移除）。
// 注意：readRequired 必须在维护操作之前（门禁事前强制：必读技能未加载时 create_*/update_* 被拦）。
func maintainPlays(ch int, nextPhase string) []play {
	return []play{
		readRequired(skillsFor("single", "maintain")...),
		{tool: "get_characters", args: `{"brief":true}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","status":"突破金丹"}]}`},
		{tool: "get_timeline", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "get_scenes", args: `{"brief":true}`, result: fmt.Sprintf(`{"scenes":[{"id":10,"title":"秘境初探","summary":"陈昊入秘境","word_count":%d}]}`, simChapterTarget[ch-1])},
		{tool: "get_item_occurrences", args: `{"item_id":3}`, result: `{"occurrences":[{"chapter_id":1,"action":"获得玉佩"},{"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"陈昊获得玉佩"}]}`},
		{tool: "get_character_relations", args: `{}`, result: `{"relations":[{"a":"陈昊","b":"林雪","relation":"师姐弟"}]}`},
		{tool: "search_lore", args: `{"query":"秘境"}`, result: `{"matches":[{"title":"青云秘境","category":"地点","content":"宗门试炼之地"}]}`},
		{tool: "search_items", args: `{"query":"玉佩"}`, result: `{"matches":[{"name":"玉佩","owner":"陈昊","narrative_role":"伏笔"}]}`},
		{tool: "update_chapter_meta", args: fmt.Sprintf(`{"chapter_id":%d,"summary":"秘境初探","key_events":["入秘境","遇仇敌","破金丹"],"characters_in":["陈昊","林雪"],"arc_ids":[1]}`, ch), result: "已更新"},
		{tool: "update_writing_snapshot", args: fmt.Sprintf(`{"last_chapter_num":%d,"summary":"第 %d 章完成"}`, ch, ch), result: "已更新"},
		{tool: "update_chapter_plan", args: `{"scope":"near","content":"第三章计划：宗门大比"}`, result: "已更新"},
		{tool: "create_scene", args: `{"chapter_id":` + fmt.Sprintf("%d", ch) + `,"scene_number":1,"title":"秘境初探","summary":"陈昊进入秘境遭遇仇敌","location_id":5,"character_ids":[1]}`, result: "已创建"},
		{tool: "update_character", args: `{"character_id":1,"status":"突破金丹","location_id":5}`, result: "已更新"},
		{tool: "update_arc_node", args: `{"node_id":3,"status":"done"}`, result: "已更新"},
		{tool: "create_timeline_entry", args: `{"title":"仇敌结怨","category":"foreshadowing","target_chapter":12,"importance":"high"}`, result: "已创建"},
		{tool: "update_timeline_entry", args: `{"entry_id":5,"resolved_chapter_id":` + fmt.Sprintf("%d", ch) + `}`, result: "已回收伏笔"},
		{tool: "update_reader_perspective_entry", args: `{"entry_id":7,"content":"玉佩与陈昊身世有关","type":"suspense"}`, result: "已更新"},
		{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"玉佩易主给林雪"}`, result: "已记录物品流转"},
		{tool: "update_character_relationship", args: `{"character_a":1,"character_b":2,"relation":"并肩作战","relation_describe":"秘境中共患难"}`, result: "已更新角色关系"},
		{tool: "edit", args: editArgs("goink.md", fmt.Sprintf("第 %d 章完成：陈昊突破金丹，玉佩新线索。当前主线：登天之路。", ch)), result: "已更新 goink.md"},
		{tool: "set_phase", args: fmt.Sprintf(`{"phase":"%s"}`, nextPhase), result: fmt.Sprintf(`{"success":true,"phase":"%s"}`, nextPhase)},
	}
}

// ── 批量门禁流程（语义化入口）──
// 全部共享 batchCore 一个构造器，对外按方案命名，避免参数泥潭。
// 质量节奏（白金方法论"三章一轮"）：自检 cadence 取 3 的倍数；
// 批量 <6 章建议 batchFullCycle（覆盖 100%），≥6 章建议 batchInCheck（覆盖 100% 且 maintain 只 1 次）。

// batchAsIs 批量现状：outline 一次出 N 章 → write 循环（无自检）→ 统一 review（只审第 1 章）+ 统一 maintain。
func batchAsIs(chapters int) []play { return batchCore(chapters, 0, 0, 0) }

// batchAsIsBase 批次循环变体：baseChapter 为本章批的起始章号偏移
// （多批循环时每批写 chapters/{base+1}..{base+chapters}，避免覆盖前批）。
func batchAsIsBase(chapters, baseChapter int) []play { return batchCore(chapters, 0, 0, baseChapter) }

// batchLightSelfCheck 批量 + 轻量自检：每 every 章插入 selfReviewPlays（2 技能 + 1 修改），不跳阶段。
func batchLightSelfCheck(chapters, every int) []play { return batchCore(chapters, 1, every) }

// batchInCheck 批量 + 批内检查：每 every 章走阶段切换 set_phase("review") → 子代理审最近 N 章 + 修复
// + 字数复查 → set_phase("write") 回写（write→review next 推进，review→write visited 回退，
// phase_gate.go:380）；统一 review + 统一 maintain 收尾。
func batchInCheck(chapters, every int) []play { return batchCore(chapters, 2, every) }

// batchFullCycle 批量 + 完整门禁循环：每个批次边界（含末尾剩余章）走 review + maintain
// （write→review→maintain→write 阶段链），无统一收尾——批量 <6 章时覆盖 100% 的唯一方案。
func batchFullCycle(chapters, every int) []play { return batchCore(chapters, 3, every) }

// batchEndReview 批量 + 批末全批审稿（简单方案，业界"重型低频"做法）：
// 现状流程不变（每章 miniMaintain + 字数校验 + 读大纲 = 轻量质量机制），
// 仅把批末统一 review 从"只审第 1 章"改为"覆盖全批"——子代理 fork 完整主历史
// （正文天然在上下文中），主会话查 N 修 N（每章 read + 修复 + 字数复查）。
// 零阶段切换、零技能重复注入。
func batchEndReview(chapters int) []play { return batchCore(chapters, 4, 0) }

// batchLightEndReview 批量 + 业界标准组合（轻量高频 + 重型低频）：
// 每 3 章轻量状态对照自检（batchLightCheckPlays，一致性向——普通用户抓不住文笔，
// 但设定矛盾一眼穿帮）+ 批末全批审稿（子代理覆盖全批）。零阶段切换。
func batchLightEndReview(chapters int) []play { return batchCore(chapters, 5, 3) }

// batchLightEndReviewBase 续写批量版：章号从 base+1 起（历史 base 章后写新批）。
func batchLightEndReviewBase(chapters, base int) []play { return batchCore(chapters, 5, 3, base) }

// batchLightCheckPlays 每 N 章轻量自检（一致性 + 文笔双向）：
// 1) 一致性（重点，普通用户一眼穿帮）：get_characters/get_timeline/get_writing_snapshot 读状态，
//    对照最近 N 章正文检查设定矛盾（角色状态不符、时间线错乱、伏笔矛盾、章节衔接断裂、重复）
// 2) 文笔（次重点）：read revision-pass + anti-ai-grade 查节奏/AI 味
// 发现问题 edit 修复。门禁：get_*/read/edit 都在 write 白名单 ✓；
// edit 事前技能强制（write 4 必读已在阶段入口注入）✓。与业界 check_consistency 同模式。
func batchLightCheckPlays(chStart, chEnd int) []play {
	return []play{
		readSkill("main-tech-revision-pass"),
		readSkill("sub-tech-anti-ai-grade"),
		{tool: "get_characters", args: `{"brief":true}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","status":"突破金丹"},{"id":2,"name":"林雪","desc":"师姐","relation":"师姐弟"}]}`},
		{tool: "get_timeline", args: fmt.Sprintf(`{"current_chapter":%d}`, chEnd), result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_writing_snapshot", args: `{}`, result: fmt.Sprintf(`{"last_chapter_num":%d,"current_arc_id":1,"current_location":"青云宗","active_chars":["陈昊","林雪"]}`, chEnd)},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, chEnd), result: chapterBodies[chEnd-1][0] + chapterBodies[chEnd-1][1]},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "一致性修复：角色状态/时间线/伏笔与 DB 状态对齐。"), result: "已修复一致性矛盾"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "文笔修复：润色过渡，去除 AI 味用词。"), result: "已修复文笔问题"},
	}
}

// batchCore 批量门禁流程核心构造器（内部实现，外部用上面的语义化入口）。
// checkKind: 0=无自检 / 1=轻量自检(selfReviewPlays) / 2=批内检查(子代理审最近 N 章) / 3=完整门禁循环(review+maintain)
// checkEvery: 自检节奏（章数，0=不检查；3 的倍数对齐白金"三章一轮"）
func batchCore(chapters int, checkKind int, checkEvery int, baseChapter ...int) []play {
	base := 0
	if len(baseChapter) > 0 {
		base = baseChapter[0]
	}
	var plays []play
	plays = append(plays, preparePlays(base+1)...) // 续写批量（base>0）从 base+1 章准备，base=0 时不变

	// outline：一次性出 N 章大纲（连续 edit，require 只查 edit 存在）
	plays = append(plays,
		readRequired(skillsFor("batch", "outline")...),
		readSkill("main-tech-book-outline"),
		readSkill("main-tech-chapter-opening"),
		readSkill("main-tech-maliang-method"),
		readSkill("main-tech-dialogue-subtext"),
		readSkill("main-tech-emotional-arc"),
		readSkill("main-tech-emotion-injection"),
		readSkill("main-type-xuanhuan-cultivation"),
	)
	for i := 1; i <= chapters; i++ {
		ch := base + i
		plays = append(plays,
			play{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), outlineText(ch)), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
			play{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), "## 关键事件\n1. 主角闯入秘境\n2. 遭遇袭击\n3. 突破瓶颈\n\n## 章末钩子\n屋外传来脚步声"), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		)
	}
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`})

	// write：循环 N 章正文，每章一个显式 write 阶段边界（set_phase("write") 同阶段幂等成功，
	// 产生阶段记录；同阶段 set_phase 不重注入技能——与真实 agent.go 一致：去重只针对同阶段，
	// 真切换（outline→write）才注入。每章边界技能已在上下文，无需重复注入）。
	// 每章 write 后紧跟迷你维护（只写不查，状态实时结算），下一章能读到最新状态。
	// 批次检查插在 write 循环内，checkKind=2 走阶段切换（门禁白名单约束：run_subagent 仅在 review 阶段）。
	for i := 1; i <= chapters; i++ {
		ch := base + i
		if ch == base+1 {
			plays = append(plays, writePlays(ch)...)
		} else {
			// 第 2+ 章：显式 write 阶段边界（同阶段幂等成功，无重复注入）
			plays = append(plays,
				play{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`},
			)
			plays = append(plays, writePlaysLean(ch)...)
		}
		// 批次检查（checkKind=1）：write 循环内按节奏插入文笔向轻量自检，不跳阶段
		if checkKind == 1 && checkEvery > 0 && i%checkEvery == 0 {
			plays = append(plays, selfReviewPlays(ch)...)
		}
		// 批次检查（checkKind=5）：write 循环内按节奏插入一致性向状态对照自检，不跳阶段
		if checkKind == 5 && checkEvery > 0 && i%checkEvery == 0 {
			plays = append(plays, batchLightCheckPlays(ch-checkEvery+1, ch)...)
		}
		// 批次检查（checkKind=2）：write 循环内按节奏插入，走阶段切换
		if checkKind == 2 && checkEvery > 0 && i%checkEvery == 0 {
			plays = append(plays, batchCheckPlays(ch-checkEvery+1, ch)...)
		}
		// 批次完整流程（checkKind=3）：每个批次边界（含末尾剩余章）都走 review+maintain，
		// 替代统一 review/maintain 收尾——即"三章一批"的完整门禁循环。
		if checkKind == 3 && (i%checkEvery == 0 || i == chapters) {
			plays = append(plays, batchFullCheckPlays(ch-checkEvery+1, ch)...)
		}
		plays = append(plays, miniMaintainPlays(ch)...)
	}
	if checkKind == 3 {
		// 完整门禁流程方案：统一 review/maintain 已由各批次边界覆盖
		return plays
	}
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`})

	// review：整批统一一次。checkKind=4/5 覆盖全批（查 N 修 N），其余只审第 1 章。
	// 章号用 base+1（批次循环打点：reviewPlays(1) 的 edit chapters/001.md 会污染 simCurrentChapter）
	if checkKind == 4 || checkKind == 5 {
		plays = append(plays, reviewPlaysBatch(chapters, base)...)
	} else {
		plays = append(plays, reviewPlays(base+1)...)
	}
	// maintain：整批统一一次（13 项清单收尾核对），batch 出口回 prepare（done 已移除）
	plays = append(plays, maintainPlays(base+chapters, "prepare")...)
	return plays
}

// reviewPlaysBatch 批末审稿覆盖全批（简单方案）：子代理 fork 完整主历史（正文已在上下文中，
// 无需额外注入），主会话"查 N 修 N"——每章 read 自查 + 修复 1 处，末尾字数复查。
// 对齐单章 reviewPlays 的"子代理 + 自查重读 + 修复 + 复查"模式。base 支持续写批量
// （章号偏移，如历史 4 章后续写批量 → base=4，审稿读第 5-9 章）。
func reviewPlaysBatch(chapters, base int) []play {
	plays := []play{
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(base + chapters)},
		// 审稿核对（真机批量会话 8/8 实测序列：全套状态核对 + 一致性检查）
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","status":"突破金丹"}]}`},
		{tool: "get_timeline", args: `{}`, result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: `{}`, result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"]}`},
		{tool: "check_story_consistency", args: `{}`, result: `{"ok":true,"issues":[]}`},
	}
	for i := 1; i <= chapters; i++ {
		ch := base + i
		plays = append(plays,
			play{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, ch), result: chapterBodies[ch-1][0] + chapterBodies[ch-1][1]},
			play{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "审稿修复：调整节奏，去除 AI 味，补强章末悬念。"), result: "已修复问题"},
		)
	}
	plays = append(plays,
		play{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(base+chapters, simChapterTarget[base+chapters-1], true)},
	)
	return plays
}

// batchCheckPlays 批次完整检查（checkKind=2）：走阶段切换（门禁白名单约束）——
// set_phase("review")（write→review 为 next 推进，review 白名单含 run_subagent）
// → 子代理审最近一个批次（chStart..chEnd）+ 修复 + 字数复查
// → set_phase("write") 回 write 继续（review→write 为回退到已访问阶段，phase_gate.go:380）。
// batch review 段配置无 auto_skill_injection（不注入）；回 write 时注入 write 技能（重复注入成本）。
func batchCheckPlays(chStart, chEnd int) []play {
	return []play{
		{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`},
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(chEnd)},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, chEnd), result: chapterBodies[chEnd-1][0] + chapterBodies[chEnd-1][1]},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "批次检查修复：调整对话节奏，去除 AI 味，补充情绪铺垫。"), result: "已修复问题 1"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "批次检查修复：伏笔衔接，强化章末悬念。"), result: "已修复问题 2"},
		{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(chEnd, simChapterTarget[chEnd-1], true)},
		{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`},
	}
}

// batchFullCheckPlays 批次完整门禁流程（checkKind=3，用户提议的"三章一批完整流程"）：
// write 批次结束 → set_phase("review")（审最近 N 章）→ set_phase("maintain")（该批状态结算）
// → set_phase("write") 回 write 继续。阶段链 write→review→maintain 为 next 推进，
// maintain→write 为回退到 visited 阶段（phase_gate.go:380）。每次切回 write 注入 write 技能，
// 每次切 maintain 注入 maintain 技能（anti-repetition/foreshadow，配置内有）。
func batchFullCheckPlays(chStart, chEnd int) []play {
	plays := []play{
		{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`},
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(chEnd)},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, chEnd), result: chapterBodies[chEnd-1][0] + chapterBodies[chEnd-1][1]},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "批次检查修复：调整对话节奏，去除 AI 味，补充情绪铺垫。"), result: "已修复问题 1"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", chEnd), "批次检查修复：伏笔衔接，强化章末悬念。"), result: "已修复问题 2"},
		{tool: "get_chapter_list", args: `{"size":1}`, result: chapterListCheck(chEnd, simChapterTarget[chEnd-1], true)},
		{tool: "set_phase", args: `{"phase":"maintain"}`, result: `{"success":true,"phase":"maintain"}`},
	}
	// 该批状态结算（maintain 13 项），出口 set_phase("write") 回写
	plays = append(plays, maintainPlays(chEnd, "write")...)
	return plays
}

// miniMaintainPlays 迷你维护（业界 delta 结算）：只写不查——
// 本章事实立即入 DB（场景/角色状态/新伏笔/物品流转/关系），
// 不调用 get_*/search_* 查询（批量下上下文已有本章正文，无需确认查询）。
// 解决"批量末尾才 maintain 导致第 N 章读到第 1 章状态"的坑，
// 同时不产生查询类轮边界（写操作是增量，前缀连续）。
func miniMaintainPlays(ch int) []play {
	return []play{
		{tool: "create_scene", args: fmt.Sprintf(`{"chapter_id":%d,"scene_number":1,"title":"秘境初探","summary":"陈昊进入秘境遭遇仇敌","location_id":5,"character_ids":[1]}`, ch), result: "已创建"},
		{tool: "update_character", args: `{"character_id":1,"status":"突破金丹","location_id":5}`, result: "已更新"},
		{tool: "create_timeline_entry", args: `{"title":"仇敌结怨","category":"foreshadowing","target_chapter":12,"importance":"high"}`, result: "已创建"},
		{tool: "update_timeline_entry", args: fmt.Sprintf(`{"entry_id":5,"resolved_chapter_id":%d}`, ch), result: "已回收伏笔"},
		{tool: "create_item_occurrence", args: fmt.Sprintf(`{"item_id":3,"chapter_id":%d,"action":"玉佩易主给林雪"}`, ch), result: "已记录物品流转"},
		{tool: "update_writing_snapshot", args: fmt.Sprintf(`{"last_chapter_num":%d,"summary":"第 %d 章完成"}`, ch, ch), result: "已更新"},
	}
}

// writePlaysLean 批量 write 循环第 2 章起：技能已在上下文（阶段内不重复加载），
// 但每章 write 前仍 read 本章大纲（kernel write 阶段 read(required)，防串章），
// 然后正文 edit + 字数校验 + 物品记录。
func writePlaysLean(ch int) []play {
	return append([]play{
		play{tool: "read", args: fmt.Sprintf(`{"path":"outlines/%03d.md"}`, ch), result: outlineText(ch)},
	}, writeBodyPlays(ch)...)
}

// outlineText 生成第 ch 章大纲（模拟 edit 输出）。
// 长度按 2026-08-16 真机批量会话校准：5 章大纲单请求输出 10,986 token（≈2.2K token/章），
// 结构对齐真机大纲模板（开篇切入/场景/情感锚点/对白/钩子/感官/温度）。
func outlineText(ch int) string {
	return fmt.Sprintf(`# 第 %d 章 %s

## 开篇
动作切入——陈昊踏入秘境入口，石门在身后合拢，四周骤然安静下来。

## 场景
秘境第一层（昏暗石殿）→ 甬道（潮湿逼仄）→ 核心祭坛（开阔明亮），从压抑到豁然开朗。

## 关键事件
1. 入秘境
2. 遇仇敌
3. 破金丹

## 重点角色
陈昊（主角）：谨慎试探，遇袭后爆发
仇敌（新登场）：阴鸷，惯用暗器，话少

## 情感
紧张探索→遇袭惊变→生死一线的压迫→突破后的释然，情绪锚点为金丹凝聚时的心跳停滞

## 对白
全章对白极少——仇敌冷笑两句，陈昊全程沉默，靠动作与环境推进

## 伏笔操作
玉佩异动（新伏笔）：祭坛光芒亮起时玉佩发热，与宗门秘辛产生关联

## 章末钩子
玉佩发出异光，祭坛深处传来沉闷的机关转动声，仿佛有什么被惊醒了

## 感官
视觉（石壁苔藓、祭坛符文、金丹金光）、听觉（滴水声、机关轰鸣）、触觉（玉佩灼热、气流涌动）、嗅觉（潮土味、铁锈味）、温度（地底阴冷到金丹凝聚时的灼热）`, ch, chapterTitle(ch))
}

func chapterTitle(ch int) string {
	titles := map[int]string{2: "秘境初探", 3: "宗门大比", 4: "丹火淬体", 5: "玉佩之谜", 6: "重返青云"}
	if t, ok := titles[ch]; ok {
		return t
	}
	return "风起云涌"
}

func chapterList(ch int) string {
	var b strings.Builder
	b.WriteString(`{"chapters":[`)
	for i := 1; i <= ch; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"num":%d,"title":"%s","word_count":%d}`, i, chapterTitle(i), simMeanWords)
	}
	b.WriteString(`]}`)
	return b.String()
}

func reviewReport(ch int) string {
	return fmt.Sprintf("审读报告（第 %d 章）：\n- 结构：完整，三幕式成立\n- 优点：突破金丹段落张力足\n- 建议 1：第三段节奏偏快，补现场描写\n- 建议 2：仇敌动机需前置铺垫\n- 建议 3：玉佩异动描写与第 1 章伏笔一致\n评分：8.5/10", ch)
}

func longContext(n int) string {
	var b strings.Builder
	b.WriteString(`{"chapter":{"num":`)
	fmt.Fprintf(&b, "%d", n)
	b.WriteString(`,"title":"第二章","word_count":`)
	fmt.Fprintf(&b, "%d", simMeanWords)
	b.WriteString(`},"recent_chapters":[`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"num":%d,"title":"章","word_count":%d}`, i, simMeanWords)
	}
	b.WriteString(`],"scenes":[{`)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteString("},{")
		}
		fmt.Fprintf(&b, `"id":%d,"title":"场景","word_count":400,"characters":["陈昊"]`, i+1)
	}
	b.WriteString(`}],"characters":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"角色%d","desc":"描述%d"}`, i+1, i+1, i+1)
	}
	b.WriteString(`],"active_arcs":[{"name":"青云之路","nodes_done":1,"nodes_total":10}],"timeline":{"pending":[{"title":"玉佩之谜","target_chapter":8}],"resolved":[{"title":"玉佩","chapter":1}]},"reader":{"known":[],"suspense":[],"misconception":[]},"stats":{"total_chapters":`)
	fmt.Fprintf(&b, "%d", n)
	b.WriteString(`}}`)
	return b.String()
}

func finalAssistant(chapter int) string {
	return fmt.Sprintf("已完成第 %d 章「入门」。主角陈昊通过宗门考验获得入门资格，埋下玉佩伏笔。第三章将进入秘境，展开第一次实质战斗。全章主要事件已写入章节文件，相关角色与位置状态已维护到库。", chapter)
}

// compressionSummary 模拟压缩产生的摘要内容
func compressionSummary() string {
	return "<system-reminder>\n上下文已压缩，请根据摘要继续。\n已完成：开书设定、第一章、第二章写作与维护。进行中：第三章秘境探索。用户偏好：快节奏、斗法细节。关键设定：异火、玉佩、宗门三系。待办：第五章伏笔回收。\n</system-reminder>"
}

// simulateSubagent 模拟 run_subagent 内部请求序列，与生产 RunSubAgent 的 fork 协议一致：
// 请求 = 完整主历史（history + cur，含刚追加的 tool_call） + [身份+NS] + [user 指令]，
// 然后子 agent 内部工具循环（read 章节、查角色/伏笔/弧线）在尾部增长。
// 子 agent 消息不落库（ToAPI=false），不影响主历史。
func simulateSubagent(cache *TokenCache, history, cur []map[string]any, turn int) [][2]int64 {
	return simulateSubagentCustom(cache, history, cur, turn, false, 1)
}

// simulateSubagentTrimmed 精简版子代理：不带完整主历史，只带 fixedSystem + 本章 cur。
// 用于验证"精简子代理历史"对主会话缓存的影响。
func simulateSubagentTrimmed(cache *TokenCache, history, cur []map[string]any, turn int) [][2]int64 {
	return simulateSubagentCustom(cache, history, cur, turn, true, 1)
}

// simulateSubagentChapters 批量审稿子代理：读全批正文后再出报告。
// 真机 8/16 批量会话实证：review 子代理 fork 完整主历史后并行 read 全部 5 章
// （每章 ~1.5K 字符正文，fork 后新增字节 ≈ 正文量，全部计入 miss）。
func simulateSubagentChapters(cache *TokenCache, history, cur []map[string]any, turn, chapters int) [][2]int64 {
	return simulateSubagentCustom(cache, history, cur, turn, false, chapters)
}

// simulateSubagentCustom 子代理审稿核心。trimmed=true 时精简历史；chapters>1 时子代理读全批。
func simulateSubagentCustom(cache *TokenCache, history, cur []map[string]any, turn int, trimmed bool, chapters int) [][2]int64 {
	var results [][2]int64

	var sub []map[string]any
	if trimmed {
		// 精简版：只带固定前缀 + 本章 cur（不含完整历史）
		sub = append([]map[string]any{}, fixedSystem()...)
		sub = append(sub, cur...)
	} else {
		// 完整版：fork 完整主历史 + 本章 cur（与 RunSubAgent 相同）
		sub = append(append([]map[string]any{}, history...), cur...)
	}
	sub = append(sub,
		map[string]any{"role": "system", "content": agentcfg.AgentIdentity(agentcfg.ReviewAgent)},
	)
	if subSkillsText != "" {
		sub = append(sub, map[string]any{"role": "system", "content": subSkillsText})
	}
	sub = append(sub,
		map[string]any{"role": "system", "content": novelState(turn)},
		map[string]any{"role": "user", "content": "请审阅最新章节：检查结构、逻辑、伏笔回收、AI 味，输出审读报告与修改建议。"},
	)

	// 子 agent 内部工具调用：fork 完整主历史（正文/writing_context 已在上下文中），
	// 只需少量定向核对（真机 8/8 实测：审稿子代理查询 ~200-700 字符/次）+
	// check_story_consistency 自动核对 + 输出审读报告。
	// 批量审稿（chapters>1）：子代理 read 全批正文（真机 8/16 实证并行 read 5 章）。
	subPlays := []play{}
	if chapters > 1 {
		for c := 1; c <= chapters; c++ {
			subPlays = append(subPlays, play{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md","start_line":1,"end_line":100}`, c), result: chapterBodies[c-1][0] + chapterBodies[c-1][1]})
		}
	} else {
		subPlays = append(subPlays, play{tool: "read", args: `{"path":"chapters/007.md","start_line":1,"end_line":100}`, result: chapterBodies[6][0] + chapterBodies[6][1]})
	}
	subPlays = append(subPlays,
		play{tool: "get_characters", args: `{"brief":true,"size":10}`, result: `{"characters":[{"id":1,"name":"陈昊","status":"突破金丹"}]}`},
		play{tool: "get_timeline", args: fmt.Sprintf(`{"current_chapter":%d}`, turn), result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		play{tool: "check_story_consistency", args: `{}`, result: `{"ok":true,"issues":[]}`},
	)
	for i, sp := range subPlays {
		hit, miss := cache.StepRaw(sub)
		results = append(results, [2]int64{hit, miss})
		sub = append(sub,
			asstToolCall(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, sp.args),
			toolMsg(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, playResult(sp)),
		)
	}
	// 子 agent 最终回复（审读报告）
	hit, miss := cache.StepRaw(sub)
	results = append(results, [2]int64{hit, miss})
	sub = append(sub, asstText(reviewReport(turn)))

	return results
}

