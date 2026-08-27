package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client 是 LLM 流式调用的传输层。
// 持有多 provider，Run 时根据 providerName 选择合适的后端。
type Client struct {
	mu        sync.RWMutex
	providers map[string]Provider // providerName → 完整配置（由 Merge 产出）
	http      *http.Client
	logger    *slog.Logger
}

// noCacheKeyProviders 记忆已确认不支持 prompt_cache_key 参数的端点。
// 部分自定义端点（如英伟达 stepfun）不忽略未知参数而是直接 400 拒绝，
// 首次命中后自动降级（不再发送该参数），避免每次请求都被拒。
var noCacheKeyProviders sync.Map // providerName → bool

// providerDisablesCacheKey 判断该 provider 是否已确认不支持 prompt_cache_key。
func providerDisablesCacheKey(providerName string) bool {
	_, ok := noCacheKeyProviders.Load(providerName)
	return ok
}

// NewClient 创建 LLM 客户端。providers 应为 Merge 产出的已组装配置。
func NewClient(providers map[string]Provider, log *slog.Logger) *Client {
	return &Client{
		providers: providers,
		http: newHTTPClient(0), // 流式请求不设超时，由 ctx 控制
		logger: log,
	}
}

// Reload 热更新 providers 配置。不影响正在进行的 ChatStream（值拷贝）。
func (c *Client) Reload(providers map[string]Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providers = providers
}

// ChatStream 发起流式对话，返回 SSE 事件 channel。
//
// providerName 指明使用哪个 Provider，messages/tools 由调用方按 OpenAI Function Calling 格式组装。
// Client 负责拼装完整 payload 并注入流式参数和钩子处理。
//
// ctx 取消时底层 HTTP 连接被中止，channel 随之关闭。
func (c *Client) ChatStream(
	ctx context.Context,
	providerName string,
	messages []map[string]any,
	tools []map[string]any,
	model string,
	opts *CallOptions,
) <-chan StreamEvent {
	c.mu.RLock()
	p, ok := c.providers[providerName]
	c.mu.RUnlock()
	if !ok {
		ch := make(chan StreamEvent, 1)
		ch <- StreamEvent{Type: EventError, Error: fmt.Errorf("unknown provider: %s", providerName)}
		close(ch)
		return ch
	}

	ch := make(chan StreamEvent, 8)
	go func() {
		defer close(ch)

		// send 构建 payload 并发送请求。cacheKey 空串表示不发送 prompt_cache_key。
		send := func(cacheKey string) (*http.Response, error) {
			var callOpts *CallOptions
			if opts != nil {
				cp := *opts
				cp.CacheKey = cacheKey
				callOpts = &cp
			}
			payload := c.buildPayload(&p, messages, tools, model, callOpts)
			body, err := marshalPayload(payload)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.ChatURL, bytes.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("failed to create HTTP request: %w", err)
			}
			// 组装请求头
			headers := map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + p.APIKey,
			}
			if p.BuildHeaders != nil {
				headers = p.BuildHeaders(headers)
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			return c.http.Do(req)
		}

		// 已确认不支持 cache key 的 provider 直接不带该参数
		initialKey := ""
		if opts != nil && !providerDisablesCacheKey(providerName) {
			initialKey = opts.CacheKey
		}

		resp, err := send(initialKey)
		if err != nil {
			ch <- StreamEvent{Type: EventError, Error: &APIError{
				StatusCode: 0,
				Message:    fmt.Sprintf("request failed: %s", err),
				Retryable:  true,
			}}
			return
		}

		// HTTP 错误
		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			// 400 且拒绝 prompt_cache_key：自动降级重发一次（不带该参数）并记忆该端点。
			// 部分端点不忽略未知参数而是直接报错，不降级则对话永远失败。
			if resp.StatusCode == http.StatusBadRequest && initialKey != "" &&
				(bytes.Contains(errBody, []byte("prompt_cache_key")) || bytes.Contains(errBody, []byte("Unsupported parameter"))) {
				noCacheKeyProviders.Store(providerName, true)
				c.logger.Warn("端点不支持 prompt_cache_key，自动降级重发（后续请求不再发送该参数）", "provider", providerName)
				resp, err = send("")
				if err != nil {
					ch <- StreamEvent{Type: EventError, Error: &APIError{
						StatusCode: 0,
						Message:    fmt.Sprintf("request failed: %s", err),
						Retryable:  true,
					}}
					return
				}
				if resp.StatusCode >= 400 {
					errBody, _ = io.ReadAll(resp.Body)
					resp.Body.Close()
				} else {
					defer resp.Body.Close()
					c.parseSSE(ch, resp.Body)
					return
				}
			}
			msg := parseDefaultError(errBody).Error()
			if p.ParseError != nil {
				msg = p.ParseError(errBody).Error()
			}
			ch <- StreamEvent{Type: EventError, Error: &APIError{
				StatusCode: resp.StatusCode,
				Message:    msg,
				Retryable:  statusRetryable(resp.StatusCode),
			}}
			return
		}

		defer resp.Body.Close()
		// SSE 逐行解析
		c.parseSSE(ch, resp.Body)
	}()

	return ch
}

// buildPayload 组装 LLM API 请求体。
// 工具定义放在 messages 之前（固定前缀），确保工具定义始终命中缓存。
func (c *Client) buildPayload(
	p *Provider,
	messages []map[string]any,
	tools []map[string]any,
	model string,
	opts *CallOptions,
) map[string]any {
	// 不使用 map（字段顺序随机），而是按固定前缀顺序组装 JSON
	// 顺序：tools(固定)→model→messages(动态)→stream→stream_options→...
	// 工具定义在消息之前，始终是公共前缀的一部分，确保缓存命中
	payload := map[string]any{
		"model":          model,
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}

	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if opts != nil && opts.ToolChoice != nil {
		payload["tool_choice"] = opts.ToolChoice
	}
	// prompt_cache_key：OpenAI 兼容缓存路由粘性（opencode 同款做法，默认对 openai-compatible 发送）。
	// 相同前缀 + 相同 key 被路由到同一后端节点，避免负载均衡漂移导致偶发全 miss；
	// 不支持的端点会忽略未知参数
	if opts != nil && opts.CacheKey != "" {
		payload["prompt_cache_key"] = opts.CacheKey
	}

	// 从 ModelInfo 取模型默认值
	var um *ModelInfo
	for i := range p.Models {
		if p.Models[i].ID == model {
			um = &p.Models[i]
			break
		}
	}

	temperature := 0.7
	if p.Temperature != nil {
		temperature = *p.Temperature
	}
	// 兜底 8192：自定义模型漏配 MaxOutputTokens 时，2500-4000 字正文 + thinking 需要足够输出空间
	maxTokens := 8192
	if opts != nil && opts.Temperature != nil {
		temperature = *opts.Temperature
	}
	if opts != nil && opts.MaxTokens != nil {
		maxTokens = *opts.MaxTokens
	} else if um != nil && um.MaxOutputTokens > 0 {
		maxTokens = um.MaxOutputTokens
	}
	payload["temperature"] = temperature
	payload["max_tokens"] = maxTokens

	// 推理/思考参数：支持思考的模型默认开启，opts 可覆盖
	thinkingEnabled := false
	reasoningEffort := ""
	if um != nil && um.SupportsThinking {
		thinkingEnabled = true
		if len(um.ReasoningLevels) > 0 {
			reasoningEffort = um.ReasoningLevels[0]
		}
	}
	if opts != nil {
		if opts.ThinkingEnabled != nil {
			thinkingEnabled = *opts.ThinkingEnabled
		}
		if opts.ReasoningEffort != nil {
			reasoningEffort = *opts.ReasoningEffort
		}
	}

	if thinkingEnabled {
		payload["thinking"] = map[string]string{"type": "enabled"}
		if reasoningEffort != "" {
			payload["reasoning_effort"] = reasoningEffort
		}
	}

	// Provider 钩子改造
	if p.BuildRequest != nil {
		payload = p.BuildRequest(payload)
	}

	// 调试：记录钩子改写后实际下发的思考参数（post-hook wire 值）。
	// 用于排查降档失效/过度思考：DB 与 messages 表只存 opts 设定值（改写前），
	// 此处才能看到模型真正收到的 reasoning_effort / thinking.type。
	if c.logger != nil {
		re, _ := payload["reasoning_effort"].(string)
		thinkingType := ""
		if t, ok := payload["thinking"].(map[string]string); ok {
			thinkingType = t["type"]
		}
		c.logger.Debug("llm request thinking params (post-hook)",
			"provider", p.Name, "model", model,
			"reasoning_effort", re, "thinking_type", thinkingType)
	}

	return payload
}

// marshalPayload 序列化 payload，工具定义放在最前（固定前缀，确保缓存命中）。
// Go 的 map 序列化按字母序排字段，tools 会被排在 messages 之后 → 消息变化时 tools 不命中缓存。
// 这里把 tools 提取到开头，让工具定义始终是公共前缀的一部分。
func marshalPayload(payload map[string]any) ([]byte, error) {
	toolsVal, hasTools := payload["tools"]
	if hasTools {
		delete(payload, "tools")
	}
	restBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if !hasTools {
		return restBytes, nil
	}
	toolsBytes, err := json.Marshal(toolsVal)
	if err != nil {
		return nil, err
	}
	// restBytes = {"model":...,"messages":...,...} → 插入 tools 到开头
	// {"tools":<toolsBytes>,<rest 去掉第一个 {>
	return []byte(`{"tools":` + string(toolsBytes) + `,` + string(restBytes[1:])), nil
}

// streamIdleTimeout 流中无新数据判定为半开连接的阈值。
// 注意：本地 Ollama 长上下文推理首 token 可能耗时 1-2 分钟，首行宽限用 streamFirstLineTimeout。
const (
	streamIdleTimeout      = 120 * time.Second
	streamFirstLineTimeout = 300 * time.Second
)

// errStreamIdle 流式响应停滞错误（读取超时）。
var errStreamIdle = fmt.Errorf("stream idle timeout")

// parseSSE 解析 SSE 流，产出 StreamEvent。
// 带 idle 超时：60s 无任何数据则报可重试错误，避免 provider 半开连接时对话永久挂起。
func (c *Client) parseSSE(ch chan<- StreamEvent, body io.Reader) {
	reader := bufio.NewReader(body)
	lineCh := make(chan struct {
		line []byte
		err  error
	}, 1)

	idleTimer := time.NewTimer(streamFirstLineTimeout)
	defer idleTimer.Stop()

	// readLine 每次启动一个读 goroutine（阻塞在 ReadBytes 上），
	// body 关闭后返回错误自动退出，无长期泄漏。
	readLine := func() ([]byte, error) {
		go func() {
			line, err := reader.ReadBytes('\n')
			lineCh <- struct {
				line []byte
				err  error
			}{line, err}
		}()
		select {
		case res := <-lineCh:
			return res.line, res.err
		case <-idleTimer.C:
			return nil, errStreamIdle
		}
	}

	// 工具调用累积缓冲区：按 index 对齐
	type accToolCall struct {
		id        string
		name      string
		arguments strings.Builder
	}
	accumulated := make([]accToolCall, 0, 4)
	hasContent := false // 追踪是否产出了有效事件
	streamStarted := false

	for {
		line, err := readLine()
		if err != nil {
			if err == errStreamIdle {
				timeoutSec := int(streamIdleTimeout.Seconds())
				if !streamStarted {
					timeoutSec = int(streamFirstLineTimeout.Seconds())
				}
				ch <- StreamEvent{Type: EventError, Error: &APIError{
					StatusCode: 0,
					Message:    fmt.Sprintf("流式响应停滞（%ds 无数据），连接可能半开", timeoutSec),
					Retryable:  true,
				}}
			} else if err != io.EOF {
				ch <- StreamEvent{Type: EventError, Error: &APIError{
					StatusCode: 0,
					Message:    fmt.Sprintf("SSE stream read error: %s", err),
					Retryable:  true,
				}}
			}
			break
		}
		// 每次成功读行都重置 idle 计时；首行宽限期（streamFirstLineTimeout）过后统一收窄到流中阈值
		idleTimer.Reset(streamIdleTimeout)
		streamStarted = true

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		// SSE data 行
		const prefix = "data: "
		if !strings.HasPrefix(lineStr, prefix) {
			continue
		}
		data := lineStr[len(prefix):]

		// 流结束标记
		if data == "[DONE]" {
			break
		}

		// 解析 JSON chunk
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			c.logger.Warn("SSE JSON parse failed", "line", data, "error", err)
			continue
		}

		// 提取 usage（可能出现在最后一个 chunk）
		if usage, ok := chunk["usage"].(map[string]any); ok && usage != nil {
			ch <- StreamEvent{Type: EventUsage, Usage: usage}
		}

		// 提取 choices
		choices, ok := chunk["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			continue
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}

		// 思考内容 → EventThinking。字段名因厂商/网关而异（无统一标准）：
		// DeepSeek/Kimi = reasoning_content；vLLM/OpenRouter/Vercel 等网关 = reasoning；
		// 部分实现 = thinking。逐个探测，自定义/网关模型思考块丢失多半是方言不匹配。
		for _, key := range []string{"reasoning_content", "reasoning", "thinking"} {
			if reasoning, ok := delta[key].(string); ok && reasoning != "" {
				ch <- StreamEvent{Type: EventThinking, Data: reasoning}
				break
			}
		}

		// content → EventContent
		if content, ok := delta["content"].(string); ok && content != "" {
			ch <- StreamEvent{Type: EventContent, Data: content}
			hasContent = true
		}

		// tool_calls delta → 按 index 累积
		toolCalls, ok := delta["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, tcRaw := range toolCalls {
			tc, _ := tcRaw.(map[string]any)
			if tc == nil {
				continue
			}
			idxFloat, ok := tc["index"].(float64)
			if !ok || idxFloat < 0 {
				c.logger.Warn("tool call delta missing valid index", "delta", tc)
				continue
			}
			idx := int(idxFloat)

			// 扩展累积缓冲区
			for len(accumulated) <= idx {
				accumulated = append(accumulated, accToolCall{})
			}
			acc := &accumulated[idx]

			// ID（首次出现）
			if id, ok := tc["id"].(string); ok && id != "" && acc.id == "" {
				acc.id = id
				ch <- StreamEvent{
					Type:  EventToolCallStart,
					Delta: &ToolCallDelta{ToolID: id},
				}
			}

			// function 子对象
			fn, ok := tc["function"].(map[string]any)
			if !ok {
				continue
			}

			// 名称（首次出现）
			if name, ok := fn["name"].(string); ok && name != "" && acc.name == "" {
				acc.name = name
				ch <- StreamEvent{
					Type:  EventToolCallStart,
					Delta: &ToolCallDelta{ToolName: name, ToolID: acc.id},
				}
			}

			// arguments 增量追加
			if args, ok := fn["arguments"].(string); ok && args != "" {
				acc.arguments.WriteString(args)
				ch <- StreamEvent{
					Type: EventToolCallArguments,
					Delta: &ToolCallDelta{
						ToolName:      acc.name,
						ToolID:        acc.id,
						ArgumentsText: acc.arguments.String(),
					},
				}
			}
		}
	}

	// 流结束后，发送完整工具调用。参数保留原始 JSON，由 Registry 按目标类型反序列化。
	for i := range accumulated {
		acc := &accumulated[i]
		if acc.name == "" || acc.arguments.Len() == 0 {
			continue
		}
		raw := acc.arguments.String()
		if !json.Valid([]byte(raw)) {
			c.logger.Warn("tool arguments JSON invalid", "tool", acc.name, "raw", raw)
			continue
		}
		ch <- StreamEvent{
			Type: EventToolCallEnd,
			Delta: &ToolCallDelta{
				ToolName:      acc.name,
				ToolID:        acc.id,
				ArgumentsText: raw,
				ArgumentsJSON: json.RawMessage(raw),
			},
		}
		hasContent = true
	}

	// 零产出检测：流结束但未收到任何有效内容，可能是服务商返回了非标准响应
	if !hasContent {
		ch <- StreamEvent{Type: EventError, Error: &APIError{
			StatusCode: 0,
			Message:    "流式响应为空，服务商可能不支持流式请求或返回了非标准格式",
			Retryable:  true,
		}}
	}
}

// statusRetryable 判断 HTTP 状态码对应的错误是否可重试。
// 429（限流）、408（超时）、5xx（服务端错误）可重试。
func statusRetryable(code int) bool {
	if code == 429 || code == 408 {
		return true
	}
	if code >= 500 && code < 600 {
		return true
	}
	return false
}

// parseDefaultError 按 OpenAI 标准格式解析错误响应体。
func parseDefaultError(body []byte) error {
	var resp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Error.Message == "" {
		return fmt.Errorf("request failed: %s", string(body))
	}
	return fmt.Errorf("%s", resp.Error.Message)
}
