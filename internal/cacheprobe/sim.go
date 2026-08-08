// cacheprobe 缓存命中率字节级探针（无网络、无 LLM 调用、无需 API Key）。
//
// 原理：DeepSeek/商汤等提供"字节精确前缀匹配"的磁盘缓存（官方文档确认：
// 命中 = 本次请求与上次请求的字节级公共前缀，TTL 内有效）。探针用真实的
// 消息序列化（Go map + encoding/json，键序确定）构造每次 LLM 调用请求，
// 按字节公共前缀计算理论命中量，对照两种 NovelState 注入协议：
//
//	go run ./cmd/cacheprobe now      # 修复后：NS 落库（本 repo 当前行为）
//	go run ./cmd/cacheprobe legacy   # 修复前：NS 动态注入不落库（旧行为）
//
// 两个场景：短问答 5 轮、门禁创作 5 轮（prepare→outline→write→review→maintain
// 每轮 20 次工具调用）。输出每次调用的 hit/miss 与累计命中率。
// 折算：1 token ≈ 4 字节（中文约 3-4 字节/字，近似）。
package cacheprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"novel/internal/agentcfg"
	"novel/internal/config"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/novel"
	"novel/internal/platform"
	"novel/internal/skill"
)

// ---- 消息级缓存模拟器（复刻 provider 语义 + tiktoken 精确计数） ----

// TokenCache 以"消息"为单位模拟前缀缓存：
// - 连续性判定：消息序列的字节级公共前缀（精确，消息内容相同则字节相同）
// - token 统计：每条消息用 tiktoken 精确计数（llm.CountMessageTokens），
//   命中 = 公共前缀覆盖的消息 token 和，miss = 其余消息 token 和
// - tools 定义作为第一条固定前缀消息参与计数
type TokenCache struct {
	prevBytes []byte // 上次请求的完整字节（连续性判定）
	prevMsgs  []map[string]any
	hit       int64 // token
	miss      int64 // token
	// transform 发送前变换（clean 方案用）：每次 Step 前对完整消息序列做处理，
	// 如把已消费的 skill 全文替换为占位符。变换后字节才参与前缀判定——
	// 这样"滑出保留窗口"是唯一前缀断裂点，连续性好。
	transform func([]map[string]any) []map[string]any
}

// msgTokens 计算单条消息的精确 token 数（tool_calls/tool_call_id/reasoning 计入）。
// 消息内容不变则 token 数不变——用缓存避免每条历史消息被重复 tiktoken 编码（性能关键：
// 400+ 次请求 × 上千条历史消息，不缓存约 8 万次 Encode，缓存后每个唯一消息只算一次）。
var msgTokenCache sync.Map // key: 消息 JSON 字符串 → int token 数

func msgTokens(m map[string]any) int {
	b, err := json.Marshal(m)
	if err != nil {
		return 0
	}
	key := string(b)
	if v, ok := msgTokenCache.Load(key); ok {
		return v.(int)
	}
	n, err := llm.CountMessageTokens(m)
	if err != nil {
		return 0
	}
	msgTokenCache.Store(key, n)
	return n
}

// toolDefs 序列化缓存（工具定义不变，避免每次请求重复 marshal + 编码）。
var (
	toolsJSONOnce    sync.Once
	toolsJSONCached  []byte
	toolsTokenCached int64
)

func cachedToolsJSON() ([]byte, int64) {
	toolsJSONOnce.Do(func() {
		toolsJSONCached, _ = json.Marshal(toolDefs)
		n, _ := llm.CountTokens(string(toolsJSONCached))
		toolsTokenCached = int64(n)
	})
	return toolsJSONCached, toolsTokenCached
}
func NewTokenCache() *TokenCache { return &TokenCache{} }

// WithCleanTransform 启用发送前清理变换：把 read/read_required 的 skill 全文
// 替换为占位符，仅保留最近 retain 条完整结果（retain<0 = 不清理）。
// 变换在 Step 内对完整请求执行（含 history+cur），保留窗口滑动时前缀断裂一次。
func NewCleanCache(retain int) *TokenCache {
	c := &TokenCache{}
	if retain >= 0 {
		c.transform = func(msgs []map[string]any) []map[string]any {
			return cleanVersion(msgs, retain)
		}
	}
	return c
}


// requestTokens 计算完整请求的 token 数：tools 前缀（固定 system 消息）+ 各消息。
func requestTokens(messages []map[string]any) (int64, int64) {
	_, toolsN := cachedToolsJSON()
	var msgsN int64
	for _, m := range messages {
		msgsN += int64(msgTokens(m))
	}
	return toolsN, msgsN
}

// Step 每次 LLM 调用。返回 (hit, miss) token 数。
// 连续性判定用字节公共前缀；token 统计按消息级精确计数。
// 主 Agent 请求：应用 transform（clean 方案）。
func (c *TokenCache) Step(messages []map[string]any) (int64, int64) {
	return c.step(messages, true)
}

// StepRaw 不应用 transform（子代理/压缩摘要保持全文，对齐真实实现：
// 子代理审稿需要读 skill 原文，压缩摘要需要全量上下文）。
func (c *TokenCache) StepRaw(messages []map[string]any) (int64, int64) {
	return c.step(messages, false)
}

func (c *TokenCache) step(messages []map[string]any, applyTransform bool) (int64, int64) {
	if applyTransform && c.transform != nil {
		messages = c.transform(messages)
	}
	toolsN, msgsN := requestTokens(messages)
	total := toolsN + msgsN

	// tools 前缀固定，始终作为第一条；消息数组整体字节用于连续性判定
	reqBytes := promptBytes(messages)

	if c.prevBytes == nil {
		c.miss += total
		c.prevBytes = reqBytes
		c.prevMsgs = append([]map[string]any{}, messages...)
		return 0, total
	}

	lcp := longestCommonPrefix(c.prevBytes, reqBytes)
	// 由字节公共前缀反推覆盖了多少条消息：逐条累加字节长度，直到超出 lcp
	hitMsgs := int64(0)
	covered := 0
	var acc int
	prefix := []byte(`[{"role":"system","content":`)
	toolsJSON, _ := json.Marshal(toolDefs)
	acc += len(prefix) + len(toolsJSON) + 2 // tools 前缀消息本身
	if acc > lcp {
		// 连 tools 前缀都没完全命中（正常不会发生，tools 固定）
		hitMsgs = 0
	} else {
		for _, m := range messages {
			b, err := json.Marshal(m)
			if err != nil {
				break
			}
			acc += 1 + len(b) // 逗号 + 消息体
			if acc > lcp {
				break
			}
			hitMsgs += int64(msgTokens(m))
			covered++
		}
	}

	hit := toolsN + hitMsgs
	miss := total - hit
	c.hit += hit
	c.miss += miss
	c.prevBytes = reqBytes
	c.prevMsgs = append([]map[string]any{}, messages...)
	return hit, miss
}

// Reset 压缩重建：丢弃整个链，此后首次调用全 miss。
func (c *TokenCache) Reset() {
	c.prevBytes = nil
	c.prevMsgs = nil
	c.hit = 0
	c.miss = 0
}

func (c *TokenCache) TotalTokens() int64 { return c.hit + c.miss }

func longestCommonPrefix(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// ---- 请求序列化（模拟服务端解析后的 token 前缀顺序） ----

var toolDefs []map[string]any

// simStore 全局技能存储（loadSystemTexts 构造），readSkill/readRequired 从它取技能正文。
var simStore *skill.Store

func initTools() {
	r := mcp_tools.NewRegistry(slog.New(slog.DiscardHandler))
	mcp_tools.RegisterAllTools(r)
	toolDefs = r.OpenAI(nil)
	initBody()
	loadSystemTexts()
}

// skillContent 从全局 SkillStore 读取技能正文（与真实 read/read_required 工具行为一致：
// 三层查找 novel > user > builtin）。Store 不可用时回退仓库文件（仅开发环境）。
func skillContent(name string) string {
	if simStore != nil {
		if sk, ok := simStore.Get(0, name); ok {
			return sk.RawContent
		}
	}
	if p, err := filepath.Abs(filepath.Join(repoRoot(), "internal", "skill", "builtin", name+".md")); err == nil {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return "（技能内容不可用: " + name + "）"
}

// promptBytes 模拟服务端把请求解析成 token 前缀后的顺序：
// tools 定义（转 system 前缀）→ 各消息顺序追加。新增消息在末尾，
// 前缀连续（与 DeepSeek/OpenAI 的 KV cache 前缀匹配语义一致）。
// msgJSONCache 缓存单条消息的序列化结果（历史消息 marshal 结果不变，避免重复序列化）。
var msgJSONCache sync.Map // key: 消息 JSON 字符串 → []byte

func msgJSON(m map[string]any) []byte {
	b0, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	key := string(b0)
	if v, ok := msgJSONCache.Load(key); ok {
		return v.([]byte)
	}
	msgJSONCache.Store(key, b0)
	return b0
}

func promptBytes(messages []map[string]any) []byte {
	toolsJSON, _ := cachedToolsJSON()
	var buf bytes.Buffer
	buf.WriteString(`[{"role":"system","content":`)
	buf.Write(toolsJSON)
	buf.WriteString(`}`)
	for _, m := range messages {
		buf.WriteByte(',')
		buf.Write(msgJSON(m))
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

// ---- 消息构造 ----

func sysMsg(content string) map[string]any {
	return map[string]any{"role": "system", "content": content}
}
func userMsg(content string) map[string]any {
	return map[string]any{"role": "user", "content": content}
}
func asstText(content string) map[string]any {
	return map[string]any{"role": "assistant", "content": content}
}
// asstToolCall 构造 assistant 工具调用消息，对齐真实 ToAPIFormat：
// 恒有 reasoning_content（空串，thinking 关闭）+ tool_displays（displayText 摘要）。
func asstToolCall(id, name, args string) map[string]any {
	return map[string]any{
		"role":    "assistant",
		"content": "",
		"reasoning_content": "",
		"tool_calls": []any{map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": args},
		}},
		"tool_displays": []any{map[string]any{
			"tool_id": id, "tool_name": name,
			"display_text":  name + " " + truncateArgs(args),
			"activity_kind": "tool",
			"phase":         "completed",
		}},
	}
}

// truncateArgs 截断 args 为短摘要（对齐真实 displayText 的展示习惯）。
func truncateArgs(args string) string {
	if len(args) > 60 {
		return args[:60] + "…"
	}
	return args
}
func toolMsg(id, name, content string) map[string]any {
	return map[string]any{
		"role": "tool", "tool_call_id": id, "name": name, "content": content,
	}
}

// ---- 固定前缀（与 writeSystemMessages 对应的三条 system，内容取自真实生成器） ----

var (
	identityText string // agentcfg.AgentIdentity(MainAgent)（真实）
	alwaysText   string // agentcfg.BuildAlwaysSkillsContent（真实，扫描 mode: always）
	catalogText  string // agentcfg.BuildSkillCatalog（真实，扫描 mode: auto）
	subSkillsText string // sub- 前缀技能拼接（review 子代理注入用，与 agent.buildSubagentSkills 同源）
)

// loadSystemTexts 用真实生成器构造固定前缀：
// identity/always/catalog 与 app/chat.go writeSystemMessages 完全一致，
// subSkills 与 internal/agent buildSubagentSkills 一致（扫描 sub- 前缀）。
// 这样模拟与真实请求同源，技能清单变动（新增/合并 skill）自动同步，零硬编码。
func loadSystemTexts() {
	store, err := skill.NewStore(slog.Default(), "")
	if err != nil {
		store = nil
	}
	var meta []skill.SkillMeta
	if store != nil {
		meta = store.ListMeta(0)
		identityText = agentcfg.AgentIdentity(agentcfg.MainAgent)
		alwaysText = agentcfg.BuildAlwaysSkillsContent(meta, store, 0)
		catalogText = agentcfg.BuildSkillCatalog(store.ListMetaForCatalog(meta))
		subSkillsText = buildSubSkills(meta, store)
	} else {
		identityText = `你是 goink 小说创作系统的主创作助手。`
		alwaysText = ""
		catalogText = ""
		subSkillsText = ""
	}
}

// buildSubSkills 扫描 sub- 前缀技能并拼接内容（与 internal/agent buildSubagentSkills 同逻辑）。
func buildSubSkills(meta []skill.SkillMeta, store *skill.Store) string {
	var b strings.Builder
	for _, m := range meta {
		if !strings.HasPrefix(m.Name, "sub-") {
			continue
		}
		sk, ok := store.Get(0, m.Name)
		if !ok {
			continue
		}
		b.WriteString("--- ")
		b.WriteString(sk.Name)
		b.WriteString(" ---\n")
		b.WriteString(sk.RawContent)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

// repoRoot 返回项目根目录（库包在 internal/cacheprobe/sim.go，向上三级；cmd 时代向上两级）。
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// file = <root>/internal/cacheprobe/sim.go → 向上三级到根
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// readFileText 读文件正文（去掉 YAML frontmatter），失败返回 fallback。
// 路径相对仓库根解析，保证 go run 与 go test 行为一致。
func readFileText(path string, max int) string {
	b, err := os.ReadFile(filepath.Join(repoRoot(), path))
	if err != nil {
		return "（读文件失败: " + err.Error() + "）"
	}
	s := string(b)
	if idx := strings.Index(s, "---\n"); idx >= 0 {
		s = s[idx+4:]
		if idx2 := strings.Index(s, "---\n"); idx2 >= 0 {
			s = s[idx2+4:]
		}
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// readFilesText 拼接多个内置 skill 的内容（read_required 的返回）。
func readFilesText(names []string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString("--- ")
		b.WriteString(n)
		b.WriteString(" ---\n")
		b.WriteString(skillContent(n))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func fixedSystem() []map[string]any {
	// 与真实 writeSystemMessages 一致：空串跳过（identity/always/catalog 各自独立消息）
	var msgs []map[string]any
	for _, c := range []string{identityText, alwaysText, catalogText} {
		if c != "" {
			msgs = append(msgs, sysMsg(c))
		}
	}
	return msgs
}

// novelState 模拟 NS，与真实 agentcfg.NovelState 字节格式完全一致：
//   【小说基础信息】书名/类型/简介 ← 真实 DB（novels 表第一本，经 GOINK_DATA_DIR/GOINK_DB_PATH）
//   当前进度：第 N 章 ← turn 动态（模拟创作推进；真实为 DB chapters 计数）
//   【章节指纹（最近）】← 真实 goink.md 尾部 1500 字符（platform.DataDir()/novels/{id}/goink.md）
// DB/文件缺失时回退占位，保证相对比较不受影响。
func novelState(turn int) string {
	var b strings.Builder
	b.WriteString(agentcfg.NovelStatePrefix)
	title, genre, desc := readRealNovelMeta()
	if title == "" {
		title, genre, desc = "焚天志", "东方玄幻", "少年秦烈身怀异火，踏入万界，快意恩仇。"
	}
	fmt.Fprintf(&b, "书名：%s\n", title)
	if genre != "" {
		fmt.Fprintf(&b, "类型：%s\n", genre)
	}
	if desc != "" {
		fmt.Fprintf(&b, "简介：%s\n", desc)
	}
	// 进度锚点：turn 动态（真实为 DB chapters 计数；模拟创作推进）
	fmt.Fprintf(&b, "当前进度：第 %d 章。创作须服务于全书总纲（book-outline.md），只展开本卷情节，后续卷设定不得提前使用。\n", turn)

	// 真实 goink.md 指纹（若存在）
	real := readRealGoinkFingerprint()
	if real != "" {
		b.WriteString("\n【章节指纹（最近）】\n")
		r := []rune(real)
		if len(r) > 1500 {
			b.WriteString(string(r[len(r)-1500:]))
			b.WriteString("\n…（更早指纹已截断，如需完整内容用 read(goink.md)）\n")
		} else {
			b.WriteString(real)
			if !strings.HasSuffix(real, "\n") {
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	// 回退占位指纹（每章 6 段指纹，约 120 字符/章，模拟 1500 字符上限）
	b.WriteString("\n【章节指纹（最近）】\n")
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "### 第%d章 %s\n\n开篇：%s\n\n场景：%s\n\n情感：%s\n\n对白：%s\n\n钩子：%s\n\n感官：%s\n\n",
			i, "秘境初探", "动作开场", "宗门演武", "紧张", "冲突对话", "悬念", "视觉听觉")
	}
	return b.String()
}

// readRealNovelMeta 从真实 DB 读取第一本小说的 书名/类型/简介（与 agentcfg.NovelState 同源）。
// DB 路径：GOINK_DB_PATH > GOINK_DATA_DIR/novel-agent.db > exe 目录 > ~/Goink。
func readRealNovelMeta() (title, genre, desc string) {
	db, err := openRealDB()
	if err != nil {
		return "", "", ""
	}
	defer db.DB()

	var n novel.Novel
	if err := db.First(&n).Error; err != nil {
		return "", "", ""
	}
	return n.Title, n.Genre, n.Description
}

// openRealDB 打开真实 DB（只读）。gorm sqlite driver。
var realDBOnce sync.Once
var realDB *gorm.DB

func openRealDB() (*gorm.DB, error) {
	realDBOnce.Do(func() {
		path := os.Getenv("GOINK_DB_PATH")
		if path == "" {
			path = filepath.Join(platform.DataDir(), "novel-agent.db")
		}
		realDB, _ = gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		})
	})
	if realDB == nil {
		return nil, fmt.Errorf("open real db failed")
	}
	return realDB, nil
}

// readRealGoinkFingerprint 读取真实 goink.md 尾部最近 1500 字符（与 agentcfg.NovelState 的 maxGoinkChars 一致）。
// 查找路径：GOINK_DATA_DIR 或 exe 目录或 ~/Goink 下的 novels/{1..N}/goink.md（取存在的一本）。
func readRealGoinkFingerprint() string {
	dir := platform.DataDir()
	novelsDir := filepath.Join(dir, "novels")
	entries, err := os.ReadDir(novelsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(novelsDir, e.Name(), "goink.md")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return string(b)
	}
	return ""
}

// ---- 短问答场景 ----

// 每轮消息构造策略（两种协议的核心差异）：
//   now（修复后）：NS 随 user 落库 → 进入历史；请求 = 历史（含全部 NS）+ 新 user + 新 NS
//   legacy（修复前）：NS 不落库 → 历史无 NS；当轮请求 = 历史 + user + 当轮 NS（紧跟 user）
// 两种模式下 NS 都紧跟当轮 user 消息（旧实现把 loadAPIMessages 结果 append 后交给
// agent 循环，NS 在 user 之后、工具循环之前，且当轮内保持）。唯一差异是历史是否保留 NS。


func shortAnswer() string {
	return "此界修炼体系：引气 → 筑基 → 金丹 → 元婴 → 化神。主角以异火之拥独特路径修行，宗门以灵石为货币。"
}

// ---- 门禁创作场景 ----

// play 一次工具调用（assistant tool_call + tool 结果）
type play struct {
	tool   string
	args   string
	result string
}

// 章节正文（固定种子，可复现）：按 app_config 设置的章节字数范围生成
// （目标字数 = min + (max-min)/2，拆成 6 段逐次 edit 追加——对应 write 阶段真实写作量级）。
var chapterBody []string

// 章节字数配置（从真实 DB app_config 读取，模拟失败回退默认 2500-4000）。
var (
	simMinWords    int
	simMaxWords    int
	simTargetWords int
	simSegLen      int
)

// readRealWordRange 从真实 DB 读章节字数上下限（get_chapter_list 校验用）。
// DB 路径与 readRealNovelMeta 相同（GOINK_DB_PATH > GOINK_DATA_DIR > exe 目录 > ~/Goink）。
func readRealWordRange() (int, int) {
	db, err := openRealDB()
	if err != nil {
		return 2500, 4000
	}
	var s config.AppSettings
	if err := db.First(&s, 1).Error; err != nil {
		return 2500, 4000
	}
	minW, maxW := s.MinChapterWords, s.MaxChapterWords
	if minW < 100 || maxW <= minW {
		return 2500, 4000
	}
	return minW, maxW
}

func initBody() {
	simMinWords, simMaxWords = readRealWordRange()
	simTargetWords = simMinWords + (simMaxWords-simMinWords)/2
	simSegLen = simTargetWords / 6

	rng := rand.New(rand.NewSource(42))
	const chars = "天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏金木水火土风雨雷电山海林木龙虎凤凰" +
		"云霞烟雾雪霜星辰日月阴阳乾坤八卦五行太极剑刀枪戟弓矢戈矛阵法宗派师徒心法内力武技神通" +
		"灵气灵根丹田金丹元婴化神散仙真仙金仙大罗斩妖除魔御剑飞行炼丹炼器符箓阵旗禁制秘境传承" +
		"试炼机缘造化天骄妖孽至尊大帝圣主神王主宰荒古太古远古上古中古近古百年千年万年永恒不朽" +
		"苍茫大地九霄之上万界诸天三千世界浩瀚星空无垠宇宙轮回因果宿命机缘命数气运天道法则"
	runes := []rune(chars)
	var b strings.Builder
	for i := 0; i < simTargetWords; i++ {
		b.WriteRune(runes[rng.Intn(len(runes))])
	}
	body := b.String()
	chapterBody = make([]string, 6)
	for i := 0; i < 6; i++ {
		start := i * simSegLen
		end := (i + 1) * simSegLen
		if end > len(body) {
			end = len(body)
		}
		chapterBody[i] = body[start:end]
	}
}

// editArgs 构造 edit 工具的 arguments JSON（含真实长度的 content）。
func editArgs(path, content string) string {
	b, err := json.Marshal(map[string]any{
		"path":        path,
		"change_type": "append",
		"content":     content,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// readSkill 构造一次 read 调用（读取 skill 文件，返回真实内容量级）。
func readSkill(name string) play {
	return play{
		tool:   "read",
		args:   fmt.Sprintf(`{"path":"skills/%s.md"}`, name),
		result: skillContent(name),
	}
}

// readRequired 模拟门禁 require_reads 的 read_required 工具调用（2026-08-08 起各阶段必读技能用此加载）。
func readRequired(names ...string) play {
	skills := strings.Join(names, ",")
	return play{
		tool:   "read_required",
		args:   fmt.Sprintf(`{"skills":"%s"}`, skills),
		result: readFilesText(names),
	}
}

// initScript 开书（init）流程：加载 5 个技能 + 建世界观/角色/弧线 + 写总纲 + 建卷
// （对照 main-core-writing-kernel 阶段技能表 init 行 + 卷结构规则）
func initScript() []play {
	return []play{
		readSkill("main-core-init-phase"),
		readSkill("main-tech-genre-templates"),
		readSkill("main-tech-book-outline"),
		readSkill("main-tech-character-design"),
		readSkill("main-tech-world-building-system"),
		{tool: "create_location", args: `{"name":"青云宗","type":"门派","desc":"主角所在宗门"}`, result: `{"id":1}`},
		{tool: "create_character", args: `{"name":"陆沉","desc":"主角","location_id":1}`, result: `{"id":1}`},
		{tool: "create_character", args: `{"name":"柳雪","desc":"师姐","location_id":1}`, result: `{"id":2}`},
		{tool: "create_lore", args: `{"title":"天地灵气","category":"规则","content":"灵气浓度决定修炼速度"}`, result: `{"id":1}`},
		{tool: "create_item", args: `{"name":"聚气丹","owner_id":1,"narrative_role":"道具"}`, result: `{"id":3}`},
		{tool: "create_timeline_entry", args: `{"title":"暗流涌动","category":"foreshadowing","target_chapter":8,"importance":"high"}`, result: `{"id":5}`},
		{tool: "create_preference", args: `{"category":"style","content":"文风简练、细节丰富"}`, result: `{"id":1}`},
		{tool: "create_story_arc", args: `{"name":"第一卷·崛起","arc_type":"volume","start_chapter":1,"end_chapter":20,"description":"主角入宗崛起"}`, result: `{"id":1}`},
		{tool: "edit", args: editArgs("book-outline.md", "# 全书总纲\n核心矛盾：主角从废柴逆袭对抗宗门暗流\n主角成长弧线：入宗→觉醒→崛起\n结局方向：登顶青云\n篇幅规划：3 卷 60 章"), result: "写入 book-outline.md"},
		{tool: "edit", args: editArgs("book-outline.md", "## 第一卷规划\n第 1-20 章：入宗与崛起，暗流初现\n爽点分布：每章至少 1 个\n伏笔主线：暗流涌动（第 8 章回收）"), result: "补充 book-outline.md 卷规划"},
		{tool: "set_phase", args: `{"phase":"prepare"}`, result: `{"success":true,"phase":"prepare"}`},
	}
}

// gateScript 一轮创作的完整工具剧本，严格对照 main-core-writing-kernel 阶段指令：
// prepare（9 required 查询 + lore/items + 3 技能）→ outline（7 技能 + 2 次大纲 edit）
// → write（11 技能全量 + 6 段正文 + 字数校验重写 + 物品记录）→ 自审（2 技能 + 1 次修改）
// → review（run_subagent + 自查重读 + 3 处修复 + 复查）→ maintain（7 查询 + 2 搜索
// + 11 项更新 + goink.md 指纹 + 2 技能）
func gateScript(turn int) []play {
	ch := turn + 1
	var plays []play
	plays = append(plays, preparePlays(ch)...)
	plays = append(plays, outlinePlays(ch)...)
	plays = append(plays, writePlays(ch)...)
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`})
	plays = append(plays, selfReviewPlays(ch)...)
	plays = append(plays, reviewPlays(ch)...)
	plays = append(plays, maintainPlays(ch, "prepare")...)
	return plays
}

// preparePlays 阶段 prepare：9 项必查（门禁 require 强制）+ 加载 prepare 技能。
func preparePlays(ch int) []play {
	return []play{
		{tool: "get_writing_context", args: fmt.Sprintf(`{"current_chapter":%d}`, ch), result: longContext(ch)},
		{tool: "get_chapter_list", args: `{}`, result: chapterList(ch)},
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","location_id":3},{"id":2,"name":"林雪","desc":"师姐","location_id":3}]}`},
		{tool: "get_timeline", args: `{}`, result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: `{}`, result: `{"arcs":[{"id":1,"name":"登天之路","type_zh":"主线","nodes_done":2,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "get_writing_snapshot", args: `{}`, result: fmt.Sprintf(`{"last_chapter_num":%d,"current_arc_id":1,"current_location":"青云宗","active_chars":["陈昊","林雪"]}`, ch-1)},
		{tool: "get_scenes", args: fmt.Sprintf(`{"chapter_id":%d}`, ch-1), result: `{"scenes":[{"id":9,"title":"入门测验","summary":"陈昊通过测验","word_count":1200}]}`},
		{tool: "get_preferences", args: `{}`, result: `{"preferences":[{"category":"style","content":"快节奏、斗法细节"},{"category":"taboo","content":"禁止主角圣母"}]}`},
		readRequired("main-tech-common-sense-logic"),
		readSkill("main-tech-genre-templates"),
		readSkill("main-tech-book-outline"),
		{tool: "get_lore", args: `{}`, result: `{"lore":[{"id":1,"title":"天地灵气","category":"规则","content":"灵气浓度决定修炼速度"}]}`},
		{tool: "get_items", args: `{}`, result: `{"items":[{"id":3,"name":"聚气丹","owner":"陆沉","narrative_role":"道具"}]}`},
		{tool: "set_phase", args: `{"phase":"outline"}`, result: `{"success":true,"phase":"outline"}`},
	}
}

// outlinePlays 阶段 outline：require_reads 必读（hook-enhanced + title-design）+ 其余技能按需 + 写大纲。
func outlinePlays(ch int) []play {
	return []play{
		readRequired("main-tech-chapter-hook-enhanced", "main-tech-chapter-title-design"),
		readSkill("main-tech-book-outline"),
		readSkill("main-tech-chapter-opening"),
		readSkill("main-tech-maliang-method"),
		readSkill("main-tech-dialogue-subtext"),
		readSkill("main-tech-emotional-arc"),
		readSkill("main-tech-emotion-injection"),
		readSkill("main-type-xuanhuan-cultivation"),
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), outlineText(ch)), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), "## 关键事件\n1. 主角闯入秘境\n2. 遭遇袭击\n3. 突破瓶颈\n\n## 章末钩子\n屋外传来脚步声"), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`},
	}
}

// writePlays 阶段 write：require_reads 必读（show-dont-tell + anti-ai-writing）+ 其余技能按需
// + read 本章大纲（kernel write 阶段第 2 步：read(required) 读 outlines/NNN.md，门禁 require 强制——
// 批量循环写多章时靠它锁定本章大纲，防止把别的章的大纲内容串进本章正文）+ 分段写正文。
// 不含阶段切换（由调用方决定何时转 review：single 每章转、batch 整批循环完才转）。
func writePlays(ch int) []play {
	plays := []play{
		readRequired("main-tech-show-dont-tell", "main-tech-anti-ai-writing"),
		readSkill("main-tech-info-density"),
		readSkill("main-tech-pov-purity"),
		readSkill("main-tech-shuangdian-pacing"),
		readSkill("main-tech-climax-scene"),
		readSkill("main-tech-foreshadow-cycle"),
		readSkill("main-tech-pacing-control"),
		readSkill("main-tech-scene-beats"),
		readSkill("main-tech-emotion-injection"),
		readSkill("main-tech-word-count-calibration"),
		play{tool: "read", args: fmt.Sprintf(`{"path":"outlines/%03d.md"}`, ch), result: outlineText(ch)},
	}
	plays = append(plays, writeBodyPlays(ch)...)
	return plays
}

// writeBodyPlays 正文写入 + 字数校验（单章/批量共用）：
// 6 段 edit 写满目标字数 → get_chapter_list 校验（首次欠字不达标）→ 补写 → 复查达标 → 物品记录。
func writeBodyPlays(ch int) []play {
	seg := simSegLen
	plays := make([]play, 0, 11)
	for i := 0; i < 6; i++ {
		written := (i + 1) * seg
		if written > simTargetWords {
			written = simTargetWords
		}
		plays = append(plays, play{
			tool:   "edit",
			args:   editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[i]),
			result: fmt.Sprintf("写入 %d 字，当前 %d/%d", len([]rune(chapterBody[i])), written, simTargetWords),
		})
	}
	plays = append(plays,
		play{tool: "get_chapter_list", args: `{}`, result: chapterListCheck(ch, simMinWords-100, false)},
		play{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "补写段落：主角凝视远方，回忆方才的惊险，掌心仍有余温。夜色中一道人影掠过屋檐，他握紧长剑，悄然跟了上去。"), result: fmt.Sprintf("补写 400 字，当前 %d/%d", simTargetWords, simTargetWords)},
		play{tool: "get_chapter_list", args: `{}`, result: chapterListCheck(ch, simTargetWords, true)},
		play{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"主角服用聚气丹"}`, result: "已记录"},
	)
	return plays
}

// chapterListCheck 构造 get_chapter_list 的字数校验返回。
func chapterListCheck(ch int, wordCount int, ok bool) string {
	return fmt.Sprintf(`{"check_chapter":%d,"word_count":%d,"word_count_ok":%v,"min_words":%d,"max_words":%d}`,
		ch, wordCount, ok, simMinWords, simMaxWords)
}

// selfReviewPlays 阶段 write 后自审：2 技能 + 1 次修改（kernel 阶段技能表 write后 行）。
func selfReviewPlays(ch int) []play {
	return []play{
		readSkill("main-tech-revision-pass"),
		readSkill("sub-tech-anti-ai-grade"),
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "自审修改：润色两处过渡，去除 AI 味用词。"), result: "自审完成"},
	}
}

// reviewPlays 阶段 review：run_subagent 审稿（require: run_subagent）+ 自查重读 + 3 处修复 + 字数复查。
func reviewPlays(ch int) []play {
	return []play{
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(ch)},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, ch), result: chapterBody[0] + chapterBody[1]},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：调整对话节奏，补充情绪铺垫。"), result: "已修复问题 1"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：前文伏笔在此回收，强化悬念。"), result: "已修复问题 2"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：删减冗余描写，收紧节奏。"), result: "已修复问题 3"},
		{tool: "get_chapter_list", args: `{}`, result: chapterListCheck(ch, simTargetWords, true)},
		{tool: "set_phase", args: `{"phase":"maintain"}`, result: `{"success":true,"phase":"maintain"}`},
	}
}

// maintainPlays 阶段 maintain：7 项状态查询 + 搜索防遗忘 + 6 类更新 + goink.md（require 13 项）。
// nextPhase 是阶段切换目标：single 模式回 "prepare"，batch 模式去 "done"。
func maintainPlays(ch int, nextPhase string) []play {
	return []play{
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","status":"突破金丹"}]}`},
		{tool: "get_timeline", args: `{}`, result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: `{}`, result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "get_scenes", args: fmt.Sprintf(`{"chapter_id":%d}`, ch), result: fmt.Sprintf(`{"scenes":[{"id":10,"title":"秘境初探","summary":"陈昊入秘境","word_count":%d}]}`, simTargetWords)},
		{tool: "get_item_occurrences", args: `{"item_id":3}`, result: `{"occurrences":[{"chapter_id":1,"action":"获得玉佩"},{"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"陈昊获得玉佩"}]}`},
		{tool: "get_character_relations", args: `{}`, result: `{"relations":[{"a":"陈昊","b":"林雪","relation":"师姐弟"}]}`},
		{tool: "search_lore", args: `{"query":"秘境"}`, result: `{"matches":[{"title":"青云秘境","category":"地点","content":"宗门试炼之地"}]}`},
		{tool: "search_items", args: `{"query":"玉佩"}`, result: `{"matches":[{"name":"玉佩","owner":"陈昊","narrative_role":"伏笔"}]}`},
		{tool: "update_chapter_meta", args: fmt.Sprintf(`{"chapter_id":%d,"summary":"秘境初探","key_events":["入秘境","遇仇敌","破金丹"],"characters_in":["陈昊","林雪"],"arc_ids":[1]}`, ch), result: "已更新"},
		{tool: "update_writing_snapshot", args: fmt.Sprintf(`{"last_chapter_num":%d,"summary":"第 %d 章完成"}`, ch, ch), result: "已更新"},
		{tool: "update_chapter_plan", args: `{"scope":"near","content":"第三章计划：宗门大比"}`, result: "已更新"},
		{tool: "create_scene", args: `{"chapter_id":` + fmt.Sprintf("%d", ch) + `,"scene_number":1,"title":"秘境初探","summary":"陈昊进入秘境遭遇仇敌","location_id":5,"character_ids":[1]}`, result: "已创建"},
		{tool: "update_character", args: `{"character_id":1,"status":"突破金丹","location_id":5}`, result: "已更新"},
		{tool: "update_arc_node", args: `{"node_id":3,"status":"done"}`, result: "已更新"},
		{tool: "create_timeline_entry", args: `{"title":"仇敌结怨","category":"foreshadowing","target_chapter":12,"importance":"high"}`, result: "已创建"},
		{tool: "update_timeline_entry", args: `{"entry_id":5,"resolved_chapter_id":` + fmt.Sprintf("%d", ch) + `}`, result: "已回收伏笔"},
		{tool: "update_reader_perspective_entry", args: `{"entry_id":7,"content":"玉佩与陈昊身世有关","type":"suspense"}`, result: "已更新"},
		{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"玉佩易主给林雪"}`, result: "已记录物品流转"},
		{tool: "update_character_relationship", args: `{"character_a":1,"character_b":2,"relation":"并肩作战","relation_describe":"秘境中共患难"}`, result: "已更新角色关系"},
		readRequired("main-tech-anti-repetition", "main-tech-foreshadow-cycle"),
		{tool: "edit", args: editArgs("goink.md", fmt.Sprintf("第 %d 章完成：陈昊突破金丹，玉佩新线索。当前主线：登天之路。", ch)), result: "已更新 goink.md"},
		{tool: "set_phase", args: fmt.Sprintf(`{"phase":"%s"}`, nextPhase), result: fmt.Sprintf(`{"success":true,"phase":"%s"}`, nextPhase)},
	}
}

// batchGatePlays 批量创作整批剧本（batch 门禁模式）：
// init → prepare（一次）→ outline（一次出 N 章大纲）→ write（循环 N 章正文）
// → review（统一一次）→ maintain（统一一次）→ done。
// 与单章连续 N 轮的关键差异：prepare/review/maintain 各只做一次，轮边界大幅减少，
// 历史跨章连续累积，NS 落库收益被放大。
func batchGatePlays(chapters int) []play {
	var plays []play
	plays = append(plays, preparePlays(1)...)

	// outline：一次性出 N 章大纲（连续 edit，require 只查 edit 存在）
	plays = append(plays,
		readRequired("main-tech-chapter-hook-enhanced", "main-tech-chapter-title-design"),
		readSkill("main-tech-book-outline"),
		readSkill("main-tech-chapter-opening"),
		readSkill("main-tech-maliang-method"),
		readSkill("main-tech-dialogue-subtext"),
		readSkill("main-tech-emotional-arc"),
		readSkill("main-tech-emotion-injection"),
		readSkill("main-type-xuanhuan-cultivation"),
	)
	for ch := 1; ch <= chapters; ch++ {
		plays = append(plays,
			play{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), outlineText(ch)), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
			play{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), "## 关键事件\n1. 主角闯入秘境\n2. 遭遇袭击\n3. 突破瓶颈\n\n## 章末钩子\n屋外传来脚步声"), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		)
	}
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`})

	// write：循环 N 章正文。read_required/技能只在循环开头加载一次
	// （门禁 require_reads 按阶段计，write 阶段只进入一次；后续章复用上下文）。
	for ch := 1; ch <= chapters; ch++ {
		if ch == 1 {
			plays = append(plays, writePlays(ch)...)
		} else {
			plays = append(plays, writePlaysLean(ch)...)
		}
	}
	plays = append(plays, play{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`})

	// review：整批统一一次（run_subagent + 修复）
	plays = append(plays, reviewPlays(1)...)
	// maintain：整批统一一次（13 项清单），batch 出口是 done
	plays = append(plays, maintainPlays(chapters, "done")...)
	return plays
}

// writePlaysLean 批量 write 循环第 2 章起：技能已在上下文（阶段内不重复加载），
// 但每章 write 前仍 read 本章大纲（kernel write 阶段 read(required)，防串章），
// 然后正文 edit + 字数校验 + 物品记录。
func writePlaysLean(ch int) []play {
	return append([]play{
		play{tool: "read", args: fmt.Sprintf(`{"path":"outlines/%03d.md"}`, ch), result: outlineText(ch)},
	}, writeBodyPlays(ch)...)
}

func outlineText(ch int) string {
	return fmt.Sprintf("# 第 %d 章 %s\n\n## 场景设计\n秘境初探：陈昊独自进入，遭遇仇敌偷袭，危机中突破金丹。\n\n## 关键事件\n1. 入秘境\n2. 遇仇敌\n3. 破金丹\n\n## 重点角色\n陈昊（主角）、仇敌（新登场）\n\n## 伏笔操作\n玉佩异动（新伏笔）\n\n## 章末钩子\n玉佩发出异光", ch, chapterTitle(ch))
}

func chapterTitle(ch int) string {
	titles := map[int]string{2: "秘境初探", 3: "宗门大比", 4: "丹火淬体", 5: "玉佩之谜", 6: "重返青云"}
	if t, ok := titles[ch]; ok {
		return t
	}
	return "风起云涌"
}

func chapterList(ch int) string {
	var b strings.Builder
	b.WriteString(`{"chapters":[`)
	for i := 1; i <= ch; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"num":%d,"title":"%s","word_count":%d}`, i, chapterTitle(i), simTargetWords)
	}
	b.WriteString(`]}`)
	return b.String()
}

func reviewReport(ch int) string {
	return fmt.Sprintf("审读报告（第 %d 章）：\n- 结构：完整，三幕式成立\n- 优点：突破金丹段落张力足\n- 建议 1：第三段节奏偏快，补现场描写\n- 建议 2：仇敌动机需前置铺垫\n- 建议 3：玉佩异动描写与第 1 章伏笔一致\n评分：8.5/10", ch)
}

func longContext(n int) string {
	var b strings.Builder
	b.WriteString(`{"chapter":{"num":`)
	fmt.Fprintf(&b, "%d", n)
	b.WriteString(`,"title":"第二章","word_count":`)
	fmt.Fprintf(&b, "%d", simTargetWords)
	b.WriteString(`},"recent_chapters":[`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"num":%d,"title":"章","word_count":%d}`, i, simTargetWords)
	}
	b.WriteString(`],"scenes":[{`)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteString("},{")
		}
		fmt.Fprintf(&b, `"id":%d,"title":"场景","word_count":400,"characters":["陈昊"]`, i+1)
	}
	b.WriteString(`}],"characters":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"角色%d","desc":"描述%d"}`, i+1, i+1, i+1)
	}
	b.WriteString(`],"active_arcs":[{"name":"青云之路","nodes_done":1,"nodes_total":10}],"timeline":{"pending":[{"title":"玉佩之谜","target_chapter":8}],"resolved":[{"title":"玉佩","chapter":1}]},"reader":{"known":[],"suspense":[],"misconception":[]},"stats":{"total_chapters":`)
	fmt.Fprintf(&b, "%d", n)
	b.WriteString(`}}`)
	return b.String()
}

func finalAssistant(chapter int) string {
	return fmt.Sprintf("已完成第 %d 章「入门」。主角陈昊通过宗门考验获得入门资格，埋下玉佩伏笔。第三章将进入秘境，展开第一次实质战斗。全章主要事件已写入章节文件，相关角色与位置状态已维护到库。", chapter)
}

// compressionSummary 模拟压缩产生的摘要内容
func compressionSummary() string {
	return "<system-reminder>\n上下文已压缩，请根据摘要继续。\n已完成：开书设定、第一章、第二章写作与维护。进行中：第三章秘境探索。用户偏好：快节奏、斗法细节。关键设定：异火、玉佩、宗门三系。待办：第五章伏笔回收。\n</system-reminder>"
}

// simulateSubagent 模拟 run_subagent 内部请求序列，与生产 RunSubAgent 的 fork 协议一致：
// 请求 = 完整主历史（history + cur，含刚追加的 tool_call） + [身份+NS] + [user 指令]，
// 然后子 agent 内部工具循环（read 章节、查角色/伏笔/弧线）在尾部增长。
// 子 agent 消息不落库（ToAPI=false），不影响主历史。
func simulateSubagent(cache *TokenCache, history, cur []map[string]any, turn int) [][2]int64 {
	var results [][2]int64

	// fork 完整主历史（与 RunSubAgent 相同：msgs = parentOpts.Messages + 尾部追加）
	// 消息拆分与真实一致：主历史 → [身份（常量）] → [sub-* 技能（常量，review 自动注入）] → [NS（动态）] → [指令]
	sub := append(append([]map[string]any{}, history...), cur...)
	sub = append(sub,
		map[string]any{"role": "system", "content": agentcfg.AgentIdentity(agentcfg.ReviewAgent)},
	)
	if subSkillsText != "" {
		sub = append(sub, map[string]any{"role": "system", "content": subSkillsText})
	}
	sub = append(sub,
		map[string]any{"role": "system", "content": novelState(turn)},
		map[string]any{"role": "user", "content": "请审阅最新章节：检查结构、逻辑、伏笔回收、AI 味，输出审读报告与修改建议。"},
	)

	// 子 agent 内部工具调用（读正文 + 查状态），结果量级与真实一致
	subPlays := []play{
		{tool: "read", args: `{"path":"chapters/007.md","start_line":1,"end_line":100}`, result: chapterBody[0] + chapterBody[1]},
		{tool: "read", args: `{"path":"chapters/007.md","start_line":100,"end_line":200}`, result: chapterBody[2] + chapterBody[3]},
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","status":"突破金丹"},{"id":2,"name":"林雪","relation":"师姐"}]}`},
		{tool: "get_timeline", args: `{}`, result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: `{}`, result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"]}`},
	}
	for i, sp := range subPlays {
		hit, miss := cache.StepRaw(sub)
		results = append(results, [2]int64{hit, miss})
		sub = append(sub,
			asstToolCall(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, sp.args),
			toolMsg(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, sp.result),
		)
	}
	// 子 agent 最终回复（审读报告）
	hit, miss := cache.StepRaw(sub)
	results = append(results, [2]int64{hit, miss})
	sub = append(sub, asstText(reviewReport(turn)))

	return results
}

