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

// 诊断:单章多轮 miss 构成——与 TokenCache 同路径按分类累计 miss，
// 回答"技能自动注入在 miss 中占多少"。
// 分类:skill_inject(阶段技能注入 system)/thinking(assistant reasoning_content)
//       body(正文 edit)/outline/query/update/other(含 reminder、工具结果等)
func TestDiagMissBreakdown(t *testing.T) {
	initTools()

	catOf := func(m map[string]any) string {
		role, _ := m["role"].(string)
		if role == "system" {
			content, _ := m["content"].(string)
			if strings.HasPrefix(content, "--- ") {
				return "skill_inject"
			}
			return "fixed"
		}
		if role == "assistant" {
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
		if strings.HasPrefix(name, "create_") || strings.HasPrefix(name, "update_") || name == "set_phase" || name == "read" || name == "read_required" {
			return "update"
		}
		return "other"
	}

	cache := NewTokenCache()
	cache.SetMissCat(catOf)
	buildMixedSession("auto", cache, 3, 0, 0)

	var sum int64
	for _, v := range cache.MissByCat {
		sum += v
	}
	fmt.Printf("\n单章 3 轮 miss 构成(buildMixedSession,与 table 同路径): miss=%d 分类和=%d 缺口=%d\n", cache.miss, sum, cache.miss-sum)

	fmt.Printf("\n单章 3 轮 miss 构成(与 TokenCache 同路径):\n")
	fmt.Printf("%-14s %12s %8s\n", "类别", "miss token", "占比")
	fmt.Println(strings.Repeat("-", 40))
	var grand int64
	for _, v := range cache.MissByCat {
		grand += v
	}
	for k, v := range cache.MissByCat {
		fmt.Printf("%-14s %12d %7.1f%%\n", k, v, 100*float64(v)/float64(grand))
	}
	fmt.Printf("%-14s %12d\n", "总计", grand)
	// 每章重复注入的成本（首轮 initInject 一次性，其余为各章 set_phase 注入）
	fmt.Printf("参考: initInject 单次注入=%d token; phaseInjectSkills 每章注入 prepare=%d outline=%d write=%d maintain=%d token\n",
		msgTokens(sysMsg(initInject)),
		msgTokens(sysMsg(phaseInjectSkills["prepare"])),
		msgTokens(sysMsg(phaseInjectSkills["outline"])),
		msgTokens(sysMsg(phaseInjectSkills["write"])),
		msgTokens(sysMsg(phaseInjectSkills["maintain"])))
}

// 诊断:批量模式质量自检节奏的成本对比（白金方法论"三章一轮"对照）。
// 实现方式：outline 一次出全批大纲，write 循环内每 N 章插入批次检查（不跳阶段）。
// checkKind: 0=无 / 1=轻量自检(selfReviewPlays) / 2=完整批次检查(子代理审最近 N 章+修复)
// 单章 5 轮作基准。回答"批量省钱的同时把质量节奏拉近单章，代价是多少"。
func TestDiagBatchSelfReview(t *testing.T) {
	initTools()

	runBatch := func(checkKind, checkEvery int) (hit, miss, out int64) {
		cache := NewTokenCache()
		plays := batchGatePlaysWith(5, checkKind, checkEvery)
		history := append([]map[string]any{}, fixedSystem()...)
		cur := []map[string]any{userMsg("请批量创作 5 章：先出全部大纲，再逐章写正文，全部完成后统一审稿与维护。")}
		cur = append(cur, sysMsg(novelState(0)))
		for i, p := range plays {
			cache.Step(append(append([]map[string]any{}, history...), cur...))
			id := fmt.Sprintf("call_b_p%d", i)
			if p.tool == "set_phase" {
				simPhase = p.args
				if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
					cur = append(cur, sysMsg(sk))
				}
				cur = append(cur, phaseReminder(p.args, true))
			}
			cur = append(cur, asstToolCall(id, p.tool, p.args))
			if p.tool == "run_subagent" {
				simulateSubagent(cache, history, cur, 5)
			}
			cur = append(cur, toolMsg(id, p.tool, p.result))
		}
		cur = append(cur, asstText("批量创作完成"))
		cache.Step(append(append([]map[string]any{}, history...), cur...))
		return cache.hit, cache.miss, cache.output
	}

	rate := func(h, m int64) float64 { return 100 * float64(h) / float64(h+m) }
	cost := func(h, m, out int64) float64 { return float64(h)*0.02/1e6 + float64(m)*1.0/1e6 + float64(out)*2.0/1e6 }

	modes := []struct {
		name     string
		kind, every int
	}{
		{"现状(攒批统一审)", 0, 0},
		{"每章轻量自检", 1, 1},
		{"三章一轮·轻量自检", 1, 3},
		{"三章一轮·完整批次检查", 2, 3},
		{"三章一轮·完整门禁流程(review+maintain)", 3, 3},
	}
	fmt.Printf("\n批量 5 章质量节奏对比（now 协议, DeepSeek 价, 单章 5 轮基准 ¥0.2751/章）:\n")
	for _, md := range modes {
		h, m, out := runBatch(md.kind, md.every)
		c := cost(h, m, out)
		save := (1 - c/5/0.2751) * 100
		fmt.Printf("  %-24s: hit=%d miss=%d out=%d 命中率=%.1f%% 成本=¥%.4f (¥%.4f/章) 比单章省 %.1f%%\n",
			md.name, h, m, out, rate(h, m), c, c/5, save)
	}
}

// 诊断:批量大小 × 三章一轮的权衡——批内章数越大固定成本摊得越薄（越便宜），
// 但连续多批引入批次边界（新窗口 prepare 重来 = 额外 miss）。
// 回答"批量上限应该设多少、三章一轮是否影响"。
func TestDiagBatchSizeTradeoff(t *testing.T) {
	initTools()

	// 连续 batches 批 × chapters 章（批次边界 = 新窗口，历史保留），三章一轮轻量自检
	runBatches := func(chapters, every, batches int) (hit, miss, out int64) {
		cache := NewTokenCache()
		for b := 0; b < batches; b++ {
			plays := batchGatePlaysWith(chapters, 1, every)
			history := append([]map[string]any{}, fixedSystem()...)
			cur := []map[string]any{userMsg(fmt.Sprintf("请批量创作 %d 章（第 %d 批）。", chapters, b+1))}
			cur = append(cur, sysMsg(novelState(b*chapters)))
			for i, p := range plays {
				cache.Step(append(append([]map[string]any{}, history...), cur...))
				id := fmt.Sprintf("call_b%d_p%d", b, i)
				if p.tool == "set_phase" {
					simPhase = p.args
					if sk, ok := phaseInjectSkills[p.args]; ok && sk != "" {
						cur = append(cur, sysMsg(sk))
					}
					cur = append(cur, phaseReminder(p.args, true))
				}
				cur = append(cur, asstToolCall(id, p.tool, p.args))
				if p.tool == "run_subagent" {
					simulateSubagent(cache, history, cur, b*chapters+chapters)
				}
				cur = append(cur, toolMsg(id, p.tool, p.result))
			}
			cur = append(cur, asstText("本批创作完成"))
			cache.Step(append(append([]map[string]any{}, history...), cur...))
		}
		return cache.hit, cache.miss, cache.output
	}

	cost := func(h, m, out int64) float64 { return float64(h)*0.02/1e6 + float64(m)*1.0/1e6 + float64(out)*2.0/1e6 }
	rate := func(h, m int64) float64 { return 100 * float64(h) / float64(h+m) }

	cases := []struct {
		name     string
		ch, every, batches int
	}{
		{"单批3章(三章一轮)", 3, 3, 1},
		{"单批5章(三章一轮)", 5, 3, 1},
		{"单批6章(三章一轮)", 6, 3, 1},
		{"单批10章(三章一轮)", 10, 3, 1},
		{"2批×3章(三章一轮,批边界)", 3, 3, 2},
		{"2批×5章(三章一轮,批边界)", 5, 3, 2},
	}
	fmt.Printf("\n批量大小 × 三章一轮（now 协议, DeepSeek 价）:\n")
	for _, c := range cases {
		h, m, out := runBatches(c.ch, c.every, c.batches)
		totalCh := c.ch * c.batches
		per := cost(h, m, out) / float64(totalCh)
		fmt.Printf("  %-28s: %2d章 hit=%d miss=%d out=%d 命中率=%.1f%% 成本=¥%.4f (¥%.4f/章)\n",
			c.name, totalCh, h, m, out, rate(h, m), cost(h, m, out), per)
	}
}
