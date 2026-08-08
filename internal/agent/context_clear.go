package agent

import "strings"

// ── 发送前工具结果清理（context clearing） ────────────────
//
// 对齐 Claude Code 微压缩层（microCompact）与 Anthropic clear_tool_uses：
// 已消费的 read/read_required 技能全文替换为占位符，保留最近 keep 条完整结果。
//
// 边界（对齐业界白名单）：
//   - 只清 read/read_required 的 tool 结果（可重新获取：SkillStore 按需重读）
//   - get_*/create_*/update_*/edit 等有状态结果一律保护
//   - 只作用于主 Agent 创作请求；子代理与压缩摘要保持全文
//     （子代理审稿需要读 skill 原文，压缩摘要需要全量上下文）
//
// 注意：清理会使该位置与上一请求字节不同 → 缓存前缀在该点断裂一次，
// 之后连续。收益是历史大幅缩小（skill 全文占单章累计输入 ~71%）。
// 本函数是纯函数，不改原消息，返回新切片（无变化时返回原切片）。

const clearPlaceholderPrefix = "[已读技能内容已清理: "

// minClearableMsgs 清理触发的最小历史长度。
// 目的：首轮对话（init 阶段连续 read 5 个技能）不清理——那些 read 结果模型刚读到，
// 属于"正在消费"而非"已消费"。只有历史足够长（一轮门禁流程约 80+ 条消息，
// 20 条 < 一轮，保证至少跨过一轮边界）且 read 数超过保留窗口时才开始清。
const minClearableMsgs = 20

// clearToolResults 返回清理后的消息副本（保留最近 keep 条 read/read_required 结果）。
// keep < 0 表示不清理（开关关闭时直接短路）。
// 历史消息数 < minClearableMsgs 时不清理（首轮/短对话保护）。
func clearToolResults(messages []map[string]any, keep int) []map[string]any {
	if keep < 0 || len(messages) < minClearableMsgs {
		return messages
	}

	// 收集所有可清理的 tool 消息下标（从后向前保留 keep 条）
	idx := make([]int, 0)
	for i, m := range messages {
		role, _ := m["role"].(string)
		if role != "tool" {
			continue
		}
		name, _ := m["name"].(string)
		if name == "read_required" || name == "read" {
			idx = append(idx, i)
		}
	}
	keepFrom := 0
	if len(idx) > keep {
		keepFrom = len(idx) - keep
	}
	if keepFrom == 0 {
		return messages
	}

	out := make([]map[string]any, len(messages))
	copy(out, messages)
	for k := 0; k < keepFrom; k++ {
		i := idx[k]
		dup := make(map[string]any, len(messages[i]))
		for key, v := range messages[i] {
			dup[key] = v
		}
		name, _ := dup["name"].(string)
		dup["content"] = clearPlaceholderPrefix + name + "]"
		out[i] = dup
	}
	return out
}

// hasClearableResults 判断消息里是否存在可清理的 read/read_required 结果。
// 用于运行时的快速短路（无 skill 读取时零开销）。
func hasClearableResults(messages []map[string]any) bool {
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role != "tool" {
			continue
		}
		name, _ := m["name"].(string)
		if name == "read_required" || name == "read" {
			return true
		}
	}
	return false
}

// isSkillRead 判断 tool 消息是否为技能读取（read_required 或 read 指向 skills/）。
// 用于审计日志与测试断言。
func isSkillRead(m map[string]any) bool {
	name, _ := m["name"].(string)
	if name == "read_required" {
		return true
	}
	if name != "read" {
		return false
	}
	content, _ := m["content"].(string)
	return strings.Contains(content, "skills/")
}
