package mcp_tools_test

import (
	"encoding/json"
	"testing"

	"novel/internal/mcp_tools"
)

// ── normalizeStringArray 单元测试 ─────────────────────────

// TestNormalizeStringArray 验证 LLM 自由格式数组规整为纯字符串数组。
func TestNormalizeStringArray(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "pure string array passes through", raw: `["剑术","隐身"]`, want: []string{"剑术", "隐身"}},
		{name: "object array flattens to name", raw: `[{"name":"再生","level":"Lv.1","description":"自动修复"}]`, want: []string{"再生"}},
		{name: "object without name falls back to description", raw: `[{"description":"潜行术"}]`, want: []string{"潜行术"}},
		{name: "mixed string and object", raw: `["格斗",{"name":"飞行","description":"亚音速"}]`, want: []string{"格斗", "飞行"}},
		{name: "numbers convert to strings", raw: `[1,2.5,true]`, want: []string{"1", "2.5", "true"}},
		{name: "empty entries dropped", raw: `["a","",{"level":3},null]`, want: []string{"a"}},
		{name: "empty array ok", raw: `[]`, want: []string{}},
		{name: "invalid json rejected", raw: `["未闭合`, wantErr: true},
		{name: "non-array rejected", raw: `{"name":"x"}`, wantErr: true},
		{name: "plain text rejected", raw: `剑术`, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := mcp_tools.NormalizeStringArray(c.raw)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.raw == "" {
				if got != "" {
					t.Fatalf("empty input should return empty string, got %q", got)
				}
				return
			}
			var arr []string
			if err := json.Unmarshal([]byte(got), &arr); err != nil {
				t.Fatalf("output not valid JSON array: %v (%q)", err, got)
			}
			if len(arr) != len(c.want) {
				t.Fatalf("got %v, want %v", arr, c.want)
			}
			for i := range arr {
				if arr[i] != c.want[i] {
					t.Fatalf("got %v, want %v", arr, c.want)
				}
			}
		})
	}
}

// TestNormalizeStringArrayValue 验证已解析值（裸数组，update_location 场景）的规整。
func TestNormalizeStringArrayValue(t *testing.T) {
	got, err := mcp_tools.NormalizeStringArrayValue([]any{
		map[string]any{"name": "危险", "level": "高"},
		"神秘",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "危险" || got[1] != "神秘" {
		t.Fatalf("got %v", got)
	}

	if _, err := mcp_tools.NormalizeStringArrayValue(map[string]any{"a": 1}); err == nil {
		t.Fatal("expected error for non-array value")
	}
}

// TestFlexString 验证 tags 类字段在反序列化阶段兼容字符串和数组两种形态
//（Ch26 事故：模型传数组导致整个请求在 json.Unmarshal 阶段失败）。
func TestFlexString(t *testing.T) {
	type args struct {
		Tags mcp_tools.FlexString `json:"tags"`
	}

	// 数组形态：规整为 JSON 文本，下游 NormalizeStringArray 可正常处理
	var a args
	if err := json.Unmarshal([]byte(`{"tags":["仙侠","上古"]}`), &a); err != nil {
		t.Fatalf("array form should unmarshal: %v", err)
	}
	var arr []string
	if err := json.Unmarshal([]byte(a.Tags), &arr); err != nil || len(arr) != 2 || arr[0] != "仙侠" {
		t.Fatalf("array form should normalize to string array, got %q (err=%v)", a.Tags, err)
	}

	// 对象数组形态：取 name
	var b args
	if err := json.Unmarshal([]byte(`{"tags":[{"name":"再生","level":"Lv.1"}]}`), &b); err != nil {
		t.Fatalf("object array should unmarshal: %v", err)
	}
	if b.Tags != `["再生"]` {
		t.Fatalf("object array should flatten to name, got %q", b.Tags)
	}

	// 字符串形态：原样保留
	var c args
	if err := json.Unmarshal([]byte(`{"tags":"[\"都市\"]"}`), &c); err != nil {
		t.Fatalf("string form should unmarshal: %v", err)
	}
	if c.Tags != `[\"都市\"]` && c.Tags != `["都市"]` {
		t.Fatalf("string form should pass through, got %q", c.Tags)
	}

	// 空值
	var d args
	if err := json.Unmarshal([]byte(`{"tags":null}`), &d); err != nil || d.Tags != "" {
		t.Fatalf("null should be empty, got %q (err=%v)", d.Tags, err)
	}
}
