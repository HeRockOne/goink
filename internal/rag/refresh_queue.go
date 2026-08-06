//go:build cgo

package rag

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"novel/internal/chapter"
	"novel/internal/git"
	"novel/internal/novel"
)

// RefreshTask 是一次向量刷新任务。
type RefreshTask struct {
	NovelID       int64
	ChapterNumber int
	Content       string
	Retried       int // 已重试次数（embedding/索引失败时重新入队，最多 maxRefreshRetries 次）
}

// maxRefreshRetries 单任务最大重试次数，避免 embedding 故障时无限循环。
const maxRefreshRetries = 2

// RefreshQueue 异步管理向量刷新，支持去重和限速。
type RefreshQueue struct {
	vs         *VectorStore
	chStore    *chapter.Store
	novelStore *novel.Store
	logger     *slog.Logger

	ch      chan RefreshTask
	pending map[string]RefreshTask
	mu      sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ── 全局单例 ──────────────────────────────────────────────

var (
	rqOnce sync.Once
	rq     *RefreshQueue
)

// InitRefreshQueue 初始化全局 RefreshQueue。多次调用只生效一次。
func InitRefreshQueue(vs *VectorStore, chStore *chapter.Store, novelStore *novel.Store, logger *slog.Logger) {
	rqOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		rq = &RefreshQueue{
			vs:         vs,
			chStore:    chStore,
			novelStore: novelStore,
			logger:     logger,
			ch:         make(chan RefreshTask, 256),
			pending:    make(map[string]RefreshTask),
			ctx:        ctx,
			cancel:     cancel,
		}
	})
}

// GetRefreshQueue 返回全局 RefreshQueue，未初始化时返回 nil。
func GetRefreshQueue() *RefreshQueue {
	return rq
}

// SubmitRefresh 提交异步向量刷新任务。若 RefreshQueue 未初始化则静默跳过。
func SubmitRefresh(novelID int64, chapterNumber int, content string) {
	if rq == nil {
		return
	}
	rq.Submit(RefreshTask{NovelID: novelID, ChapterNumber: chapterNumber, Content: content})
}

// ── 实例方法 ──────────────────────────────────────────────

// Start 启动后台消费者 goroutine。
func (q *RefreshQueue) Start() {
	q.wg.Add(1)
	go q.consumer()
}

// Stop 取消后台消费者并等待完成。
func (q *RefreshQueue) Stop() {
	q.cancel()
	q.wg.Wait()
}

// Submit 提交异步向量刷新任务。队列满时等待最多 2 秒，仍满才丢弃（避免静默丢任务导致章节搜不到）。
func (q *RefreshQueue) Submit(task RefreshTask) {
	select {
	case q.ch <- task:
	case <-time.After(2 * time.Second):
		q.logger.Warn("向量刷新队列持续已满，丢弃任务", "chapter_number", task.ChapterNumber)
	}
}

func pendingKey(task RefreshTask) string {
	return fmt.Sprintf("%d:%d", task.NovelID, task.ChapterNumber)
}

// consumer 是后台消费者，500ms 内同一章节的重复提交合并为一次。
func (q *RefreshQueue) consumer() {
	defer q.wg.Done()

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	timerActive := false

	for {
		select {
		case <-q.ctx.Done():
			// 退出前清空 pending（使用独立 context，不受 cancel 影响）
			q.mu.Lock()
			pending := q.pending
			q.pending = make(map[string]RefreshTask)
			q.mu.Unlock()
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
			for _, task := range pending {
				q.doRefreshWithCtx(drainCtx, task)
			}
			drainCancel()
			return

		case task := <-q.ch:
			q.mu.Lock()
			q.pending[pendingKey(task)] = task
			if !timerActive {
				timer.Reset(500 * time.Millisecond)
				timerActive = true
			}
			q.mu.Unlock()

		case <-timer.C:
			q.mu.Lock()
			timerActive = false
			tasks := q.pending
			q.pending = make(map[string]RefreshTask)
			q.mu.Unlock()

			for _, task := range tasks {
				q.doRefresh(task)
			}
		}
	}
}

func (q *RefreshQueue) doRefresh(task RefreshTask) {
	ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)
	defer cancel()
	q.doRefreshWithCtx(ctx, task)
}

// refreshRetry 刷新失败时重新入队（有限次数），保证索引要么是新的、要么是旧的，绝不空。
func (q *RefreshQueue) refreshRetry(task RefreshTask) {
	if task.Retried >= maxRefreshRetries {
		q.logger.Error("向量刷新重试次数耗尽，章节暂无法索引", "novel_id", task.NovelID, "chapter_number", task.ChapterNumber)
		return
	}
	task.Retried++
	q.Submit(task)
}

func (q *RefreshQueue) doRefreshWithCtx(ctx context.Context, task RefreshTask) {

	ch, err := q.chStore.GetByNovelAndNumber(ctx, task.NovelID, task.ChapterNumber)
	if err != nil {
		q.logger.Warn("查章节失败，跳过向量刷新", "novel_id", task.NovelID, "chapter_number", task.ChapterNumber, "err", err)
		return
	}

	params := ChapterChunkParams{
		ChapterNumber: task.ChapterNumber,
		ChapterTitle:  ch.Title,
		Content:       task.Content,
		Summary:       ch.Summary,
	}
	chunks := BuildChapterChunks(params, GetTokenizer())
	if len(chunks) == 0 {
		return
	}

	// 先删除旧索引，再写入新索引。
	// 删除失败仅 warn（重复块会在下次刷新时被清理）
	if err := q.vs.DeleteChapterChunks(ctx, task.NovelID, task.ChapterNumber); err != nil {
		q.logger.Warn("删除章节旧向量失败（重复块下次刷新清理）", "chapter_number", task.ChapterNumber, "err", err)
	}

	// 写入新索引。失败时该章节搜不到旧内容（已删），但下次刷新会重试补齐。
	if err := q.vs.IndexChunks(ctx, task.NovelID, chunks); err != nil {
		q.logger.Error("索引章节向量失败，重试入队", "chapter_number", task.ChapterNumber, "err", err)
		q.refreshRetry(task)
	}
}

// RebuildNovel 无条件全量重建一部小说的向量索引。
// 失败时清空已建的部分索引（count 归零），保证 RebuildAll 的 count>0 跳过逻辑不会永久跳过。
func (q *RefreshQueue) RebuildNovel(ctx context.Context, novelID int64) error {
	chapters, err := q.chStore.ListAllByNovel(ctx, novelID)
	if err != nil {
		return fmt.Errorf("rag: list chapters for rebuild: %w", err)
	}

	if len(chapters) == 0 {
		return nil
	}

	q.vs.DeleteNovel(ctx, novelID)

	var batch []Chunk
	batchCount := 0
	totalChunks := 0

	for _, ch := range chapters {
		content, err := git.ReadFile(novelID, ch.FilePath)
		if err != nil {
			q.logger.Warn("读取章节文件失败，跳过", "chapter_id", ch.ID, "path", ch.FilePath, "err", err)
			continue
		}

		params := ChapterChunkParams{
			ChapterNumber: ch.ChapterNumber,
			ChapterTitle:  ch.Title,
			Content:       content,
			Summary:       ch.Summary,
		}
		chunks := BuildChapterChunks(params, GetTokenizer())
		batch = append(batch, chunks...)

		if len(batch) >= maxBatchSize {
			if err := q.vs.IndexChunks(ctx, novelID, batch); err != nil {
				q.cleanupAfterRebuildFail(ctx, novelID)
				return fmt.Errorf("rag: index batch: %w", err)
			}
			totalChunks += len(batch)
			batch = batch[:0]
			batchCount++
			if batchCount%4 == 0 {
				select {
				case <-ctx.Done():
					q.cleanupAfterRebuildFail(ctx, novelID)
					return ctx.Err()
				case <-time.After(50 * time.Millisecond):
				}
			}
		}
	}

	if len(batch) > 0 {
		if err := q.vs.IndexChunks(ctx, novelID, batch); err != nil {
			q.cleanupAfterRebuildFail(ctx, novelID)
			return fmt.Errorf("rag: index final batch: %w", err)
		}
		totalChunks += len(batch)
	}

	q.logger.Info("全量向量重建完成", "novel_id", novelID, "chapters", len(chapters), "chunks", totalChunks)
	return nil
}

// cleanupAfterRebuildFail 重建失败时删除部分索引，使 count 归零以便下次重试。
func (q *RefreshQueue) cleanupAfterRebuildFail(ctx context.Context, novelID int64) {
	if err := q.vs.DeleteNovel(ctx, novelID); err != nil {
		q.logger.Warn("重建失败清理部分索引失败（下次重建前 count 判断可能误跳过）", "novel_id", novelID, "err", err)
	}
}

// RebuildAll 遍历全部小说，对尚无向量索引的小说执行首次全量重建。
// 存量检测：向量表有数据但 FTS5 表缺失/为空（老库升级到 FTS5 前索引的章节）时也触发重建，
// 否则老章节永远进不了全文索引，关键词检索对存量数据形同虚设。
func (q *RefreshQueue) RebuildAll(ctx context.Context) error {
	var novels []novel.Novel
	if err := q.novelStore.DB.WithContext(ctx).Find(&novels).Error; err != nil {
		return fmt.Errorf("rag: list novels: %w", err)
	}

	for _, n := range novels {
		count, err := q.vs.CountChunks(ctx, n.ID)
		if err != nil {
			q.logger.Warn("检查向量行数失败，跳过", "novel_id", n.ID, "err", err)
			continue
		}
		if count == 0 {
			q.logger.Info("开始首次向量索引", "novel_id", n.ID, "title", n.Title)
			if err := q.RebuildNovel(ctx, n.ID); err != nil {
				q.logger.Error("小说向量重建失败", "novel_id", n.ID, "err", err)
				continue
			}
			continue
		}
		// 向量已有但 FTS5 缺失/为空：老库升级，重建补齐全文索引
		ftsCount, err := q.vs.FtsCount(ctx, n.ID)
		if err != nil {
			q.logger.Warn("检查 FTS 行数失败，跳过", "novel_id", n.ID, "err", err)
			continue
		}
		if ftsCount == 0 {
			q.logger.Info("检测到 FTS5 全文索引缺失，重建补齐", "novel_id", n.ID, "title", n.Title)
			if err := q.RebuildNovel(ctx, n.ID); err != nil {
				q.logger.Error("小说向量重建失败", "novel_id", n.ID, "err", err)
				continue
			}
		}
	}

	return nil
}
