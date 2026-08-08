package cacheprobe

import (
	"fmt"
	"strings"
	"testing"
)

// 诊断:门禁单章完整流程中,各类消息对总输入 token 的贡献占比。
// 分类:fixed(系统提示+tools+NS)/skill(read/read_required)/body(正文edit)/outline(大纲edit)
//       query(get_*/search_* 查询结果)/update(create_*/update_*/set_phase)/subagent/other
func TestDiagTokenBreakdown(t *testing.T) {
	initTools()

	type cat struct {
		name  string
		total int64
		count int64
	}
	catOf := func(m map[string]any) string {
		role, _ := m["role"].(string)
		if role == "system" {
			return "fixed"
		}
		// assistant 消息的 tool_calls.arguments 含正文/大纲全文(editArgs)
		if role == "assistant" {
			if tcs, ok := m["tool_calls"].([]any); ok {
				for _, tc := range tcs {
					tcm, _ := tc.(map[string]any)
					fn, _ := tcm["function"].(map[string]any)
					args, _ := fn["arguments"].(string)
					if strings.Contains(args, "chapters/") && len(args) > 100 {
						return "body"
					}
					if strings.Contains(args, "outlines/") {
						return "outline"
					}
				}
			}
			return "other"
		}
		name, _ := m["name"].(string)
		if name == "read_required" || name == "read" {
			return "skill"
		}
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
		if strings.HasPrefix(name, "create_") || strings.HasPrefix(name, "update_") || name == "set_phase" {
			return "update"
		}
		return "other"
	}

	counters := map[string]*cat{}
	ensure := func(k string) *cat {
		if counters[k] == nil {
			counters[k] = &cat{name: k}
		}
		return counters[k]
	}
	// tools 前缀与固定 system 计入 fixed(每次请求都有)
	_, toolsN := cachedToolsJSON()
	ensure("fixed").total += toolsN
	ensure("fixed").count++

	// 构造单章流程(与 buildGateWithRounds 相同,但统计每次请求的消息构成)
	cache := NewTokenCache()
	history := append([]map[string]any{}, fixedSystem()...)

	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	for i, p := range initScript() {
		req := append(append([]map[string]any{}, history...), cur...)
		cache.Step(req)
		for _, m := range req {
			ensure(catOf(m)).total += int64(msgTokens(m))
			ensure(catOf(m)).count++
		}
		cur = append(cur, asstToolCall(fmt.Sprintf("call_init_p%d", i), p.tool, p.args), toolMsg(fmt.Sprintf("call_init_p%d", i), p.tool, p.result))
		if p.tool == "set_phase" {
			cur = append(cur, phaseReminder(p.args, true))
		}
	}
	commitCur("now", &history, cur)

	plays := gateScript(1)
	for i, p := range plays {
		req := append(append([]map[string]any{}, history...), cur...)
		cache.Step(req)
		for _, m := range req {
			ensure(catOf(m)).total += int64(msgTokens(m))
			ensure(catOf(m)).count++
		}
		cur = append(cur, asstToolCall(fmt.Sprintf("call_g_p%d", i), p.tool, p.args))
		if p.tool == "run_subagent" {
			subResults := simulateSubagent(cache, history, cur, 1)
			for _, pr := range subResults {
				_ = pr
			}
			// 子代理消息统计
			sub := append(append([]map[string]any{}, history...), cur...)
			for _, m := range sub {
				ensure(catOf(m)).total += int64(msgTokens(m))
				ensure(catOf(m)).count++
			}
		}
		cur = append(cur, toolMsg(fmt.Sprintf("call_g_p%d", i), p.tool, p.result))
		if p.tool == "set_phase" {
			cur = append(cur, phaseReminder(p.args, true))
		}
	}

	var grand int64
	for _, c := range counters {
		grand += c.total
	}
	fmt.Printf("\n%-12s %12s %8s %8s\n", "类别", "累计token", "占比", "消息数")
	fmt.Println(strings.Repeat("-", 48))
	for _, c := range counters {
		pct := 0.0
		if grand > 0 {
			pct = 100 * float64(c.total) / float64(grand)
		}
		fmt.Printf("%-12s %12d %7.1f%% %8d\n", c.name, c.total, pct, c.count)
	}
	fmt.Printf("%-12s %12d %7.1f%%\n", "总计", grand, 100.0)
}
