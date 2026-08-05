//go:build cgo

package rag

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

// VectorStore 使用 sqlite-vec 管理向量索引，每部小说一张 vec0 虚拟表 + 一张 FTS5 全文表。
type VectorStore struct {
	db          *sql.DB
	embedder    Embedder
	log         *slog.Logger
	ensuredOnce sync.Map // map[int64]bool，避免重复 CREATE TABLE IF NOT EXISTS
	ftsOnce     sync.Map // map[int64]bool，FTS5 表是否可用（trigram tokenizer 不可用则禁用全文检索）
}

// NewVectorStore 创建向量存储。db 应为已启用 sqlite-vec 扩展的 SQLite 连接。
func NewVectorStore(db *sql.DB, embedder Embedder, log *slog.Logger) *VectorStore {
	return &VectorStore{db: db, embedder: embedder, log: log}
}

func (s *VectorStore) tableName(novelID int64) string {
	return fmt.Sprintf("vec_novel_%d", novelID)
}

func (s *VectorStore) ftsTableName(novelID int64) string {
	return fmt.Sprintf("fts_novel_%d", novelID)
}

// ensureFtsTable 确保 FTS5 全文表存在。优先 trigram tokenizer（中文子串匹配），
// 不可用则回退 unicode61（整词匹配，召回率降低但不至于不可用），仍失败则禁用全文检索。
func (s *VectorStore) ensureFtsTable(ctx context.Context, novelID int64) (bool, error) {
	if v, ok := s.ftsOnce.Load(novelID); ok {
		return v.(bool), nil
	}
	name := s.ftsTableName(novelID)
	create := func(tokenize string) error {
		_, err := s.db.ExecContext(ctx, fmt.Sprintf(
			`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING fts5(
				chunk_id UNINDEXED,
				content,
				chunk_type UNINDEXED,
				chapter_number UNINDEXED,
				start_position UNINDEXED,
				tokenize = '%s'
			)`, name, tokenize))
		return err
	}
	if err := create("trigram"); err != nil {
		s.log.Warn("rag: trigram tokenizer 不可用，回退 unicode61", "err", err)
		if err2 := create("unicode61"); err2 != nil {
			s.log.Error("rag: FTS5 建表失败，禁用全文检索", "err", err2)
			s.ftsOnce.Store(novelID, false)
			return false, nil
		}
	}
	s.ftsOnce.Store(novelID, true)
	return true, nil
}

// FtsSearch 在 FTS5 全文表中执行关键词检索（chunk_id 升序，用于 RRF 合并）。
func (s *VectorStore) FtsSearch(ctx context.Context, novelID int64, query string, topK int) ([]SearchResult, error) {
	ok, err := s.ensureFtsTable(ctx, novelID)
	if err != nil || !ok {
		return nil, err
	}
	matchExpr := escapeFtsQuery(query)
	if matchExpr == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT chunk_id, content, chapter_number, start_position FROM %s WHERE content MATCH ? LIMIT ?`,
			s.ftsTableName(novelID)),
		matchExpr, topK)
	if err != nil {
		return nil, fmt.Errorf("rag: fts search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunkID, content string
		var chapterNumber, startRunePos int
		if err := rows.Scan(&chunkID, &content, &chapterNumber, &startRunePos); err != nil {
			return nil, fmt.Errorf("rag: fts scan: %w", err)
		}
		results = append(results, SearchResult{
			ChunkID:       chunkID,
			Content:       content,
			ChapterNumber: chapterNumber,
			StartRunePos:  startRunePos,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rag: fts iterate: %w", err)
	}
	return results, nil
}

// escapeFtsQuery 将用户查询转义为 FTS5 安全短语。trigram/unicode61 下按子串/词匹配。
func escapeFtsQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}
	// FTS5 特殊字符：双引号、星号、冒号、括号等。用短语引号包裹并转义内部引号。
	q = strings.ReplaceAll(q, `"`, `""`)
	return `"` + q + `"`
}

// ensureTable 确保指定小说的向量表存在，不存在则创建。结果缓存在 ensuredOnce 中。
func (s *VectorStore) ensureTable(ctx context.Context, novelID int64) error {
	if _, ok := s.ensuredOnce.Load(novelID); ok {
		return nil
	}

	tableName := s.tableName(novelID)
	// 迁移：旧表无 start_position 列时重建。列不存在和查询出错需区分处理。
	hasCol, colErr := tableHasColumn(ctx, s.db, tableName, "start_position")
	if colErr != nil {
		s.log.Warn("rag: 检查列失败，跳过迁移", "table", tableName, "err", colErr)
	} else if !hasCol {
		s.log.Info("rag: 旧表缺少 start_position 列，重建中", "table", tableName)
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			return fmt.Errorf("rag: drop old vec table %s: %w", tableName, err)
		}
	}
	sql := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(
		embedding float[512] distance_metric=cosine,
		chunk_id text,
		content text,
		chunk_type text,
		chapter_number integer,
		chunk_index integer,
		start_position integer
	)`, tableName)

	_, err := s.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("rag: create vec table for novel %d: %w", novelID, err)
	}
	s.ensuredOnce.Store(novelID, true)
	return nil
}

// IndexChunks 将文本块批量生成 embedding 并在事务中写入向量表。
func (s *VectorStore) IndexChunks(ctx context.Context, novelID int64, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	if err := s.ensureTable(ctx, novelID); err != nil {
		return err
	}
	ftsOK, err := s.ensureFtsTable(ctx, novelID)
	if err != nil {
		s.log.Warn("rag: FTS5 表不可用，仅向量索引", "novel_id", novelID, "err", err)
	}

	// 批量生成 embedding，一次 ONNX Run。
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	embs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("rag: batch embed: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rag: begin tx: %w", err)
	}
	defer tx.Rollback()

	tableName := s.tableName(novelID)
	ftsName := s.ftsTableName(novelID)
	for i, chunk := range chunks {
		v, err := sqlite_vec.SerializeFloat32(embs[i])
		if err != nil {
			return fmt.Errorf("rag: serialize chunk %s: %w", chunk.ID, err)
		}

		_, err = tx.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO %s (embedding, chunk_id, content, chunk_type, chapter_number, chunk_index, start_position) VALUES (?, ?, ?, ?, ?, ?, ?)`, tableName),
			v, chunk.ID, chunk.Content, chunk.ChunkType, chunk.ChapterNumber, chunk.ChunkIndex, chunk.StartRunePos,
		)
		if err != nil {
			return fmt.Errorf("rag: insert chunk %s: %w", chunk.ID, err)
		}

		// 同步写入 FTS5 全文表（与向量同事务，保证原子一致）
		if ftsOK {
			_, err = tx.ExecContext(ctx,
				fmt.Sprintf(`INSERT INTO %s (chunk_id, content, chunk_type, chapter_number, start_position) VALUES (?, ?, ?, ?, ?)`, ftsName),
				chunk.ID, chunk.Content, chunk.ChunkType, chunk.ChapterNumber, chunk.StartRunePos,
			)
			if err != nil {
				return fmt.Errorf("rag: insert fts chunk %s: %w", chunk.ID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rag: commit tx: %w", err)
	}

	s.log.Info("向量索引完成", "novel_id", novelID, "chunks", len(chunks))
	return nil
}

// Search 在指定小说的向量索引中执行语义检索。
func (s *VectorStore) Search(ctx context.Context, novelID int64, query string, topK int, filter *SearchFilter) ([]SearchResult, error) {
	if err := s.ensureTable(ctx, novelID); err != nil {
		return nil, err
	}

	queryEmb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rag: embed query: %w", err)
	}

	q, err := sqlite_vec.SerializeFloat32(queryEmb)
	if err != nil {
		return nil, fmt.Errorf("rag: serialize query: %w", err)
	}

	tableName := s.tableName(novelID)
	whereClauses := []string{}
	args := []any{q}

	if filter != nil {
		if len(filter.ChapterNumbers) > 0 {
			placeholders := make([]string, len(filter.ChapterNumbers))
			for i, id := range filter.ChapterNumbers {
				placeholders[i] = "?"
				args = append(args, id)
			}
			whereClauses = append(whereClauses,
				fmt.Sprintf("chapter_number IN (%s)", strings.Join(placeholders, ",")))
		}
		if len(filter.ChunkTypes) > 0 {
			placeholders := make([]string, len(filter.ChunkTypes))
			for i, t := range filter.ChunkTypes {
				placeholders[i] = "?"
				args = append(args, t)
			}
			whereClauses = append(whereClauses,
				fmt.Sprintf("chunk_type IN (%s)", strings.Join(placeholders, ",")))
		}
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " AND " + strings.Join(whereClauses, " AND ")
	}

	querySQL := fmt.Sprintf(
		`SELECT chunk_id, content, chunk_type, chapter_number, distance, start_position, embedding FROM %s WHERE embedding MATCH ?%s ORDER BY distance LIMIT ?`,
		tableName, whereSQL,
	)
	args = append(args, topK)

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("rag: search query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunkID, content, chunkType string
		var chapterNumber, startRunePos int
		var distance float64
		var embBlob []byte
		if err := rows.Scan(&chunkID, &content, &chunkType, &chapterNumber, &distance, &startRunePos, &embBlob); err != nil {
			return nil, fmt.Errorf("rag: scan result: %w", err)
		}
		relevance := 1.0 - distance
		if relevance < 0 {
			relevance = 0
		}
		results = append(results, SearchResult{
			ChunkID:       chunkID,
			Content:       content,
			SourceType:    chunkType,
			ChapterNumber: chapterNumber,
			StartRunePos:  startRunePos,
			Distance:      distance,
			Relevance:     relevance,
			Embedding:     deserializeFloat32(embBlob),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rag: iterate results: %w", err)
	}

	s.log.Info("向量检索完成", "novel_id", novelID, "query_len", len([]rune(query)), "results", len(results))
	return results, nil
}

// DeleteChapterChunks 删除指定章节的所有向量块（含 FTS5 全文块）。
func (s *VectorStore) DeleteChapterChunks(ctx context.Context, novelID int64, chapterNumber int) error {
	tableName := s.tableName(novelID)
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE chapter_number = ?`, tableName),
		chapterNumber,
	); err != nil {
		return fmt.Errorf("rag: delete chapter %d chunks: %w", chapterNumber, err)
	}
	// FTS5 表存在则同步删除
	if _, ok := s.ftsOnce.Load(novelID); ok {
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE chapter_number = ?`, s.ftsTableName(novelID)),
			chapterNumber,
		); err != nil {
			s.log.Warn("rag: delete fts chapter chunks failed", "chapter_number", chapterNumber, "err", err)
		}
	}
	s.log.Info("已删除章节向量", "novel_id", novelID, "chapter_number", chapterNumber)
	return nil
}

// CountChunks 返回指定小说的向量块总数。表不存在时自动创建后返回 0。
func (s *VectorStore) CountChunks(ctx context.Context, novelID int64) (int, error) {
	if err := s.ensureTable(ctx, novelID); err != nil {
		return 0, err
	}
	var count int
	err := s.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName(novelID))).Scan(&count)
	return count, err
}

// DeleteNovel 删除整部小说的向量表与 FTS5 表。
func (s *VectorStore) DeleteNovel(ctx context.Context, novelID int64) error {
	tableName := s.tableName(novelID)
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName),
	); err != nil {
		return fmt.Errorf("rag: drop table for novel %d: %w", novelID, err)
	}
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf("DROP TABLE IF EXISTS %s", s.ftsTableName(novelID)),
	); err != nil {
		return fmt.Errorf("rag: drop fts table for novel %d: %w", novelID, err)
	}
	s.ensuredOnce.Delete(novelID)
	s.ftsOnce.Delete(novelID)
	s.log.Info("已删除小说向量表", "novel_id", novelID)
	return nil
}

// ── 全局单例 ──────────────────────────────────────────────

var (
	globalVSOnce sync.Once
	globalVS     *VectorStore
)

// InitVectorStore 初始化全局 VectorStore，多次调用只生效一次。
func InitVectorStore(db *sql.DB, embedder Embedder, log *slog.Logger) {
	globalVSOnce.Do(func() {
		globalVS = NewVectorStore(db, embedder, log)
	})
}

// tableHasColumn 检查虚拟表中是否存在指定列。vec0 不支持 PRAGMA，
// 所以通过查询后 Columns() 判断。返回 (exists, error)。
func tableHasColumn(ctx context.Context, db *sql.DB, tableName, col string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT 0", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return false, err
	}
	for _, c := range cols {
		if strings.EqualFold(c, col) {
			return true, nil
		}
	}
	return false, nil
}

// GetVectorStore 返回全局 VectorStore，未初始化时返回 nil。
func GetVectorStore() *VectorStore {
	return globalVS
}
