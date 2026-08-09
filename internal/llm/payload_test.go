package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPayloadToolsFirst 验证真实 API 请求体的工具定义在 payload 最前（固定前缀）。
func TestPayloadToolsFirst(t *testing.T) {
	p := Provider{Name: "test", ChatURL: "http://localhost", Models: []ModelInfo{{ID: "m", SupportsThinking: false}}}
	c := &Client{providers: map[string]Provider{"test": p}}
	msgs := []map[string]any{
		{"role": "system", "content": "L1 identity"},
		{"role": "system", "content": "L2 always"},
		{"role": "system", "content": "L3 catalog"},
		{"role": "user", "content": "你好"},
	}
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{"name": "get_x", "description": "x"}},
	}
	payload := c.buildPayload(&p, msgs, tools, "m", &CallOptions{})
	body, err := marshalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// 工具定义必须在最前
	if !strings.HasPrefix(s, `{"tools":`) {
		t.Fatalf("工具定义不在 payload 最前: %s", s[:50])
	}
	// 工具定义在 model 和 messages 之前
	toolsIdx := strings.Index(s, `"tools":`)
	modelIdx := strings.Index(s, `"model":`)
	msgsIdx := strings.Index(s, `"messages":`)
	if toolsIdx < 0 || modelIdx < 0 || msgsIdx < 0 {
		t.Fatal("缺少 tools/model/messages 字段")
	}
	if !(toolsIdx < modelIdx && toolsIdx < msgsIdx) {
		t.Fatalf("字段顺序错误: tools=%d model=%d messages=%d", toolsIdx, modelIdx, msgsIdx)
	}
	t.Logf("payload 格式正确: 工具定义在最前")
}

// TestPayloadJSONValid 验证 payload 是合法 JSON。
func TestPayloadJSONValid(t *testing.T) {
	p := Provider{Name: "test", ChatURL: "http://localhost", Models: []ModelInfo{{ID: "m", SupportsThinking: false}}}
	c := &Client{providers: map[string]Provider{"test": p}}
	msgs := []map[string]any{{"role": "user", "content": "hi"}}
	payload := c.buildPayload(&p, msgs, []map[string]any{{"type": "function"}}, "m", &CallOptions{})
	body, err := marshalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("payload 非法 JSON: %v", err)
	}
	if _, ok := parsed["tools"]; !ok {
		t.Fatal("缺少 tools 字段")
	}
	if _, ok := parsed["messages"]; !ok {
		t.Fatal("缺少 messages 字段")
	}
	t.Logf("payload 是合法 JSON，含 tools 和 messages")
}