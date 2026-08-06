//go:build cgo

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	bertMaxLen   = 512
	maxBatchSize = 16
)

const queryInstruction = "为这个句子生成表示以用于检索相关文章："

var (
	tokenizer *Tokenizer
	tokOnce   sync.Once
	tokErr    error
)

type OnnxEmbedder struct {
	session   *ort.DynamicAdvancedSession
	tokenizer *Tokenizer
	clsID     int
	sepID     int
	mu        sync.Mutex
	log       *slog.Logger
}

func (e *OnnxEmbedder) Dim() int { return 512 }

// onnxEnvOnce 保证 ONNX Runtime 环境进程级只初始化一次。
// onnxruntime_go 的环境是全局单例，二次 InitializeEnvironment 报
// "The onnxruntime has already been initialized"；LazyEmbedder 卸载（销毁
// session 释放模型内存）后重新加载必须复用环境，只重建 session。
var (
	onnxEnvOnce sync.Once
	onnxEnvErr  error
)

func ensureOnnxEnvironment() error {
	onnxEnvOnce.Do(func() {
		onnxEnvErr = ort.InitializeEnvironment()
	})
	return onnxEnvErr
}

func newOnnxEmbedder(modelsDir string, t *Tokenizer, log *slog.Logger) (*OnnxEmbedder, error) {
	modelPath := filepath.Join(modelsDir, "model.onnx")

	clsID, ok1 := t.vocab["[CLS]"]
	sepID, ok2 := t.vocab["[SEP]"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("rag: vocab missing [CLS] or [SEP]")
	}

	if err := ensureOnnxEnvironment(); err != nil {
		return nil, fmt.Errorf("rag: init onnx environment: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"}, nil)
	if err != nil {
		return nil, fmt.Errorf("rag: load model: %w", err)
	}

	log.Info("ONNX embedder 已初始化", "model", modelPath)
	return &OnnxEmbedder{
		session:   session,
		tokenizer: t,
		clsID:     clsID,
		sepID:     sepID,
		log:       log,
	}, nil
}

func (e *OnnxEmbedder) prepare(text string) []int {
	raw := e.tokenizer.Tokenize(text)
	limit := bertMaxLen - 2
	if len(raw) > limit {
		e.log.Warn("token 序列超过模型上限，已截断", "len", len(raw), "max", limit)
		raw = raw[:limit]
	}
	ids := make([]int, len(raw)+2)
	ids[0] = e.clsID
	copy(ids[1:], raw)
	ids[len(ids)-1] = e.sepID
	return ids
}

func (e *OnnxEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	ids := e.prepare(queryInstruction + text)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	seqLen := int64(len(ids))
	inputIDs := make([]int64, seqLen)
	attentionMask := make([]int64, seqLen)
	tokenTypeIDs := make([]int64, seqLen)
	for i, id := range ids {
		inputIDs[i] = int64(id)
		attentionMask[i] = 1
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), attentionMask)
	if err != nil {
		return nil, fmt.Errorf("rag: create mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	typeIDsTensor, err := ort.NewTensor(ort.NewShape(1, seqLen), tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create type_ids tensor: %w", err)
	}
	defer typeIDsTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, seqLen, 512))
	if err != nil {
		return nil, fmt.Errorf("rag: create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	e.mu.Lock()
	defer e.mu.Unlock()
	err = e.session.Run(
		[]ort.Value{inputTensor, maskTensor, typeIDsTensor},
		[]ort.Value{outputTensor},
	)

	if err != nil {
		return nil, fmt.Errorf("rag: onnx run: %w", err)
	}

	hidden := outputTensor.GetData()
	return clsPool(hidden, 512), nil
}

func (e *OnnxEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) <= maxBatchSize {
		return e.embedBatch(ctx, texts)
	}

	results := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := e.embedBatch(ctx, texts[i:end])
		if err != nil {
			return nil, fmt.Errorf("rag: batch [%d:%d]: %w", i, end, err)
		}
		results = append(results, batch...)
	}
	return results, nil
}

func (e *OnnxEmbedder) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tokenized := make([][]int, len(texts))
	maxLen := 0
	for i, text := range texts {
		tokenized[i] = e.prepare(text)
		if len(tokenized[i]) > maxLen {
			maxLen = len(tokenized[i])
		}
	}
	if maxLen == 0 {
		results := make([][]float32, len(texts))
		for i := range results {
			results[i] = make([]float32, 512)
		}
		return results, nil
	}

	batchSize := int64(len(texts))
	seqLen := int64(maxLen)

	inputIDs := make([]int64, batchSize*seqLen)
	attentionMask := make([]int64, batchSize*seqLen)
	tokenTypeIDs := make([]int64, batchSize*seqLen)

	for i := int64(0); i < batchSize; i++ {
		ids := tokenized[i]
		base := i * seqLen
		for j, id := range ids {
			inputIDs[base+int64(j)] = int64(id)
			attentionMask[base+int64(j)] = 1
		}
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), inputIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	maskTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), attentionMask)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	typeIDsTensor, err := ort.NewTensor(ort.NewShape(batchSize, seqLen), tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("rag: create batch type_ids tensor: %w", err)
	}
	defer typeIDsTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(batchSize, seqLen, 512))
	if err != nil {
		return nil, fmt.Errorf("rag: create batch output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	e.mu.Lock()
	defer e.mu.Unlock()
	err = e.session.Run(
		[]ort.Value{inputTensor, maskTensor, typeIDsTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("rag: batch onnx run: %w", err)
	}

	hidden := outputTensor.GetData()
	results := make([][]float32, batchSize)
	sampleSize := int(seqLen) * 512
	for i := int64(0); i < batchSize; i++ {
		start := int(i) * sampleSize
		results[i] = clsPool(hidden[start:start+sampleSize], 512)
	}

	return results, nil
}

func (e *OnnxEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.session != nil {
		e.session.Destroy()
		e.session = nil
	}
	return nil
}

// ── LazyEmbedder ──────────────────────────────────────────

type LazyEmbedder struct {
	modelsDir string
	log       *slog.Logger

	mu        sync.Mutex
	inner     *OnnxEmbedder
	idleTimer *time.Timer
	stopped   bool
}

func NewLazyEmbedder(modelsDir string, log *slog.Logger) *LazyEmbedder {
	return &LazyEmbedder{modelsDir: modelsDir, log: log}
}

func (l *LazyEmbedder) load() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner != nil {
		return nil
	}
	if l.stopped {
		return fmt.Errorf("rag: lazy embedder is closed")
	}
	t := GetTokenizer()
	if t == nil {
		return fmt.Errorf("rag: tokenizer not initialized")
	}
	e, err := newOnnxEmbedder(l.modelsDir, t, l.log)
	if err != nil {
		return err
	}
	l.inner = e
	return nil
}

func (l *LazyEmbedder) unload() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner != nil {
		l.inner.Close()
		l.inner = nil
	}
}

func (l *LazyEmbedder) resetIdleTimer() {
	if l.idleTimer != nil {
		l.idleTimer.Stop()
	}
	l.idleTimer = time.AfterFunc(2*time.Minute, l.unload)
}

func (l *LazyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := l.load(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.resetIdleTimer()
	l.mu.Unlock()
	return l.inner.Embed(ctx, text)
}

func (l *LazyEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if err := l.load(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	l.resetIdleTimer()
	l.mu.Unlock()
	return l.inner.EmbedBatch(ctx, texts)
}

func (l *LazyEmbedder) Dim() int { return 512 }

func (l *LazyEmbedder) Close() error {
	l.mu.Lock()
	l.stopped = true
	if l.idleTimer != nil {
		l.idleTimer.Stop()
	}
	l.mu.Unlock()
	l.unload()
	return nil
}

// ── Tokenizer ─────────────────────────────────────────────

func InitTokenizer(modelsDir string, log *slog.Logger) {
	tokOnce.Do(func() {
		t, err := NewTokenizer(filepath.Join(modelsDir, "vocab.txt"))
		if err != nil {
			tokErr = err
			log.Error("加载 tokenizer 失败", "err", err)
			return
		}
		tokenizer = t
		log.Info("Tokenizer 已加载")
	})
}

func GetTokenizer() *Tokenizer {
	return tokenizer
}

// ── Pooling ───────────────────────────────────────────────

func clsPool(hidden []float32, dim int) []float32 {
	vec := make([]float32, dim)
	copy(vec, hidden[:dim])
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= float32(norm)
		}
	}
	return vec
}