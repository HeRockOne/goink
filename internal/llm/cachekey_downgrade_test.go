package llm

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestPromptCacheKeyAutoDowngrade 验证 400 拒绝 prompt_cache_key 时自动降级：
// 首次请求带参数被 400（如英伟达 stepfun 端点直接报 Unsupported parameter），
// 自动去掉参数重发成功；同 provider 后续请求不再发送该参数（进程内记忆）。
func TestPromptCacheKeyAutoDowngrade(t *testing.T) {
	var reqCount atomic.Int32
	var firstHasKey, secondHasKey, thirdHasKey atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := reqCount.Add(1)
		hasKey := strings.Contains(string(body), "prompt_cache_key")
		switch n {
		case 1:
			firstHasKey.Store(hasKey)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("{\"error\":{\"message\":\"Validation: Unsupported parameter(s): `prompt_cache_key`\",\"type\":\"Bad Request\",\"code\":400}}"))
		default:
			if n == 2 {
				secondHasKey.Store(hasKey)
			} else {
				thirdHasKey.Store(hasKey)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer server.Close()

	p := Provider{Name: "stepfun", ChatURL: server.URL, Models: []ModelInfo{{ID: "m", SupportsThinking: false}}}
	c := NewClient(map[string]Provider{"stepfun": p}, slog.Default())
	msgs := []map[string]any{{"role": "user", "content": "hi"}}
	opts := &CallOptions{CacheKey: "sess_1"}

	// 第一次调用：请求1 带 key 被 400 → 自动降级重发（请求2 不带 key）→ 成功
	ch := c.ChatStream(context.Background(), "stepfun", msgs, nil, "m", opts)
	got := ""
	for ev := range ch {
		if ev.Type == EventContent {
			got += ev.Data
		}
		if ev.Type == EventError {
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}
	if got != "hi" {
		t.Fatalf("expected content 'hi', got %q", got)
	}
	if !firstHasKey.Load() {
		t.Error("first request should include prompt_cache_key")
	}
	if secondHasKey.Load() {
		t.Error("downgrade retry should NOT include prompt_cache_key")
	}

	// 第二次调用：记忆生效，直接不带 key，一次成功
	ch = c.ChatStream(context.Background(), "stepfun", msgs, nil, "m", opts)
	for ev := range ch {
		if ev.Type == EventError {
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}
	if got2 := reqCount.Load(); got2 != 3 {
		t.Fatalf("expected 3 requests total, got %d", got2)
	}
	if thirdHasKey.Load() {
		t.Error("subsequent request should NOT include prompt_cache_key")
	}
}
