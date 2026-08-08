package agent

import (
	"strings"
	"testing"
)

// clearToolResults:只清理 read/read_required 的 tool 结果,保留最近 keep 条,
// 有状态结果(get_*/create_*/update_*/edit)不受影响。
// 历史过短(首轮)不清理。
func TestClearToolResults(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "c1", "function": map[string]any{"name": "read_required", "arguments": `{"skills":"main-tech-show-dont-tell"}`}}}},
		{"role": "tool", "tool_call_id": "c1", "name": "read_required", "content": strings.Repeat("技能全文", 500)},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "c2", "function": map[string]any{"name": "get_characters", "arguments": `{}`}}}},
		{"role": "tool", "tool_call_id": "c2", "name": "get_characters", "content": `{"characters":[...]}`},
		{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "c3", "function": map[string]any{"name": "edit", "arguments": `{"path":"chapters/001.md"}`}}}},
		{"role": "tool", "tool_call_id": "c3", "name": "edit", "content": "写入正文"},
	}

	// 首轮保护:消息数 < minClearableMsgs 时不清理(即使 read 数 > keep)
	if out := clearToolResults(msgs, 0); out[2]["content"] != msgs[2]["content"] {
		t.Fatal("首轮对话不应清理 read 结果")
	}

	// 构造足够长的历史(超过 minClearableMsgs),此时才清理
	long := make([]map[string]any, 0, minClearableMsgs+3)
	long = append(long, msgs...)
	for i := 0; i < minClearableMsgs; i++ {
		long = append(long, map[string]any{"role": "user", "content": "填充消息"})
	}
	// keep=0:全部清理
	out := clearToolResults(long, 0)
	if !strings.Contains(out[2]["content"].(string), clearPlaceholderPrefix) {
		t.Fatalf("read_required 结果应被清理, got %q", out[2]["content"])
	}
	if out[4]["content"] != `{"characters":[...]}` {
		t.Fatalf("get_* 结果不应被清理: %q", out[4]["content"])
	}
	if out[6]["content"] != "写入正文" {
		t.Fatalf("edit 结果不应被清理: %q", out[6]["content"])
	}
	// 原消息不被修改
	if !strings.Contains(long[2]["content"].(string), "技能全文") {
		t.Fatal("原消息被修改了")
	}

	// keep=1:保留最近 1 条
	msgs2 := append([]map[string]any{}, long...)
	msgs2 = append(msgs2,
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "c4", "function": map[string]any{"name": "read_required", "arguments": `{"skills":"x"}`}}}},
		map[string]any{"role": "tool", "tool_call_id": "c4", "name": "read_required", "content": strings.Repeat("第二份全文", 300)},
	)
	out2 := clearToolResults(msgs2, 1)
	if strings.Contains(out2[2]["content"].(string), "技能全文") {
		t.Fatal("最早的 read 应被清理")
	}
	if !strings.Contains(out2[len(out2)-1]["content"].(string), "第二份全文") {
		t.Fatalf("最近 1 条应保留: %q", out2[len(out2)-1]["content"])
	}

	// keep=-1:不清理
	out3 := clearToolResults(long, -1)
	if !strings.Contains(out3[2]["content"].(string), "技能全文") {
		t.Fatal("keep=-1 不应清理")
	}
}

// hasClearableResults:无 read/read_required 时返回 false(短路零开销)。
func TestHasClearableResults(t *testing.T) {
	if hasClearableResults([]map[string]any{
		{"role": "tool", "name": "get_characters", "content": "x"},
		{"role": "tool", "name": "edit", "content": "y"},
	}) {
		t.Fatal("无 skill 读取不应判定为可清理")
	}
	if !hasClearableResults([]map[string]any{
		{"role": "tool", "name": "read_required", "content": "x"},
	}) {
		t.Fatal("read_required 应判定为可清理")
	}
}
