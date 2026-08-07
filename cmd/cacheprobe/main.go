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
package main

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

	"novel/internal/llm"
	"novel/internal/mcp_tools"
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
}

func NewTokenCache() *TokenCache { return &TokenCache{} }

// msgTokens 计算单条消息的精确 token 数（含 tool_calls/tool_call_id/reasoning）。
func msgTokens(m map[string]any) int {
	n, err := llm.CountMessageTokens(m)
	if err != nil {
		return 0
	}
	return n
}

// requestTokens 计算完整请求的 token 数：tools 前缀（固定 system 消息）+ 各消息。
func requestTokens(messages []map[string]any) (int64, int64) {
	toolsJSON, _ := json.Marshal(toolDefs)
	toolsN, _ := llm.CountTokens(string(toolsJSON))
	var msgsN int64
	for _, m := range messages {
		msgsN += int64(msgTokens(m))
	}
	return int64(toolsN), msgsN
}

// Step 每次 LLM 调用。返回 (hit, miss) token 数。
// 连续性判定用字节公共前缀；token 统计按消息级精确计数。
func (c *TokenCache) Step(messages []map[string]any) (int64, int64) {
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

func initTools() {
	r := mcp_tools.NewRegistry(slog.New(slog.DiscardHandler))
	mcp_tools.RegisterAllTools(r)
	toolDefs = r.OpenAI(nil)
	initBody()
	loadSystemTexts()
}

// promptBytes 模拟服务端把请求解析成 token 前缀后的顺序：
// tools 定义（转 system 前缀）→ 各消息顺序追加。新增消息在末尾，
// 前缀连续（与 DeepSeek/OpenAI 的 KV cache 前缀匹配语义一致）。
func promptBytes(messages []map[string]any) []byte {
	toolsJSON, err := json.Marshal(toolDefs)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	buf.WriteString(`[{"role":"system","content":`)
	buf.Write(toolsJSON)
	buf.WriteString(`}`)
	for _, m := range messages {
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		buf.WriteByte(',')
		buf.Write(b)
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
func asstToolCall(id, name, args string) map[string]any {
	return map[string]any{
		"role":    "assistant",
		"content": "",
		"tool_calls": []any{map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{"name": name, "arguments": args},
		}},
	}
}
func toolMsg(id, name, content string) map[string]any {
	return map[string]any{
		"role": "tool", "tool_call_id": id, "name": name, "content": content,
	}
}

// ---- 固定前缀（与 writeSystemMessages 对应的三条 system，内容取自真实文件） ----

var (
	identityText string // mainAgentSystem1（agentcfg/identity.go）
	alwaysKernel string // skills/main-core-writing-kernel.md 正文
	alwaysComm   string // skills/main-core-ai-communication-standard.md 正文
	catalogText  string // 技能目录（auto 模式 name+description 汇总）
)

// loadSystemTexts 读取真实文件构造固定前缀。失败时降级为内嵌摘要（不影响字节结构）。
func loadSystemTexts() {
	identityText = `你是 goink 小说创作系统的主创作助手，协助用户管理角色、情节、世界观和叙事结构。你可以读取小说全部数据，并通过 MCP 工具维护角色、时间线、弧线、地点、世界观、物品和读者认知。
【核心原则】创作质量第一；设定一致性由数据库保证；按阶段门禁推进 prepare→outline→write→review→maintain。
【输出规范】中文正文，杜绝 AI 味；每个工具调用前说明意图。`
	alwaysKernel = readFileText("skills/main-core-writing-kernel.md", 9000)
	alwaysComm = readFileText("skills/main-core-ai-communication-standard.md", 1000)
	catalogText = `技能目录（auto 模式，按需 read 加载）：
main-core-init-phase 开书流程；main-tech-genre-templates 12类型模板；main-tech-book-outline 总纲/卷纲/章节蓝图；
main-tech-character-design 角色设计；main-tech-world-building-system 世界观；main-tech-common-sense-logic 一致性；
main-tech-brainstorm-composer 卡情节构思；main-tech-chapter-opening 章节开头；main-tech-chapter-hook-enhanced 章末钩子；
main-tech-maliang-method 打脸节奏；main-tech-dialogue-subtext 对白设计；main-tech-emotional-arc 情感弧线；
main-tech-opening-chapter 第一章开篇；main-tech-show-dont-tell 展示而非告知；main-tech-info-density 信息密度；
main-tech-pov-purity 视角纯净；main-tech-anti-ai-writing 反AI八条铁律；main-tech-shuangdian-pacing 爽点节奏；
main-tech-climax-scene 战斗章；main-tech-foreshadow-cycle 伏笔循环；main-tech-pacing-control 节奏控制；
main-tech-scene-beats 场景节拍；main-tech-emotion-injection 情绪注入；main-tech-word-count-calibration 字数校准；
main-tech-revision-pass 修改润色；main-tech-anti-repetition 去重；main-tech-golden-three-chapters 黄金三章；
main-tech-golden-finger-design 金手指；main-tech-chapter-title-design 章节标题；main-tech-book-completion 完本清单；
main-type-xuanhuan-cultivation 玄幻；main-type-urban-martial-arts 都市；main-type-post-apocalyptic-survival 末日；
main-type-suspense-rule-horror 悬疑；main-type-historical-time-travel 历史穿越；sub-tech-anti-ai-grade 用词反AI；
sub-tech-review-standards 16项审稿判定`
}

// repoRoot 返回项目根目录（无论从 go run 还是 go test 运行，均解析到仓库根）。
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// file = <root>/cmd/cacheprobe/main.go → 向上两级
	return filepath.Dir(filepath.Dir(file))
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
		b.WriteString(readFileText("internal/skill/builtin/"+n+".md", 4000))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func fixedSystem() []map[string]any {
	return []map[string]any{
		sysMsg(identityText),
		sysMsg("【常驻技能 1/2】main-core-writing-kernel\n" + alwaysKernel),
		sysMsg("【常驻技能 2/2】main-core-ai-communication-standard\n" + alwaysComm),
		sysMsg(catalogText),
	}
}

// NS 快照：主体固定 + 轮次编号变化（模拟 goink.md 状态演进）
func novelState(turn int) string {
	return fmt.Sprintf("【小说基础信息】\n书名：焚天志\n类型：东方玄幻\n简介：少年秦烈身怀异火，踏入万界，快意恩仇。\n\n【故事状态】\n已推进至第 %d 章。主线：宗门之争。最近提交：第 %d 章完成。", turn, turn)
}

// ---- 短问答场景 ----

// 每轮消息构造策略（两种协议的核心差异）：
//   now（修复后）：NS 随 user 落库 → 进入历史；请求 = 历史（含全部 NS）+ 新 user + 新 NS
//   legacy（修复前）：NS 不落库 → 历史无 NS；当轮请求 = 历史 + user + 当轮 NS（紧跟 user）
// 两种模式下 NS 都紧跟当轮 user 消息（旧实现把 loadAPIMessages 结果 append 后交给
// agent 循环，NS 在 user 之后、工具循环之前，且当轮内保持）。唯一差异是历史是否保留 NS。

func buildShortQA(mode string, cache *TokenCache) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	for turn := 1; turn <= 5; turn++ {
		// 当轮动态消息：user + NS（两种协议都紧跟 user）
		cur := []map[string]any{userMsg(fmt.Sprintf("第 %d 问：这个世界的修炼体系是什么？", turn))}
		cur = append(cur, sysMsg(novelState(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		// 历史更新：now 含 NS；legacy 不含 NS（NS 未落库）
		if mode == "now" {
			history = append(history, cur...)
		} else {
			history = append(history, cur[:1]...)
		}
		history = append(history, asstText(shortAnswer()))
	}
	return results
}

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

// 章节正文（固定种子，可复现）：模拟真实 3000 字中文正文，
// 拆成 3 段（每段 1000 字），逐次 edit 追加——对应 write 阶段真实写作量级。
var chapterBody []string

func initBody() {
	rng := rand.New(rand.NewSource(42))
	const chars = "天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏金木水火土风雨雷电山海林木龙虎凤凰" +
		"云霞烟雾雪霜星辰日月阴阳乾坤八卦五行太极剑刀枪戟弓矢戈矛阵法宗派师徒心法内力武技神通" +
		"灵气灵根丹田金丹元婴化神散仙真仙金仙大罗斩妖除魔御剑飞行炼丹炼器符箓阵旗禁制秘境传承" +
		"试炼机缘造化天骄妖孽至尊大帝圣主神王主宰荒古太古远古上古中古近古百年千年万年永恒不朽" +
		"苍茫大地九霄之上万界诸天三千世界浩瀚星空无垠宇宙轮回因果宿命机缘命数气运天道法则"
	runes := []rune(chars)
	var b strings.Builder
	for i := 0; i < 3000; i++ {
		b.WriteRune(runes[rng.Intn(len(runes))])
	}
	body := b.String()
	chapterBody = make([]string, 6)
	for i := 0; i < 6; i++ {
		chapterBody[i] = body[i*500 : (i+1)*500]
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
		result: readFileText("internal/skill/builtin/"+name+".md", 4000),
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
	return []play{
		// ── prepare：9 项必查（门禁 require 强制）+ 加载 prepare 技能 ──
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

		// 阶段 outline：require_reads 必读（hook-enhanced + title-design）+ 其余技能按需 + 写大纲
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

		// 阶段 write：require_reads 必读（show-dont-tell + anti-ai-writing）+ 其余技能按需 + 分段写正文
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
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[0]), result: "写入 500 字，当前 500/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[1]), result: "写入 500 字，当前 1000/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[2]), result: "写入 500 字，当前 1500/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[3]), result: "写入 500 字，当前 2000/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[4]), result: "写入 500 字，当前 2500/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[5]), result: "写入 500 字，当前 3000/3000"},
		{tool: "get_chapter_list", args: `{}`, result: fmt.Sprintf(`{"check_chapter":%d,"word_count":2600,"word_count_ok":false,"min_words":2500,"max_words":4000}`, ch)},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "补写段落：主角凝视远方，回忆方才的惊险，掌心仍有余温。夜色中一道人影掠过屋檐，他握紧长剑，悄然跟了上去。"), result: "补写 400 字，当前 3000/3000"},
		{tool: "get_chapter_list", args: `{}`, result: fmt.Sprintf(`{"check_chapter":%d,"word_count":3012,"word_count_ok":true,"min_words":2500,"max_words":4000}`, ch)},
		{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"主角服用聚气丹"}`, result: "已记录"},
		{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`},

		// 阶段 write 后自审：2 技能 + 1 次修改（kernel 阶段技能表 write后 行）
		readSkill("main-tech-revision-pass"),
		readSkill("sub-tech-anti-ai-grade"),
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "自审修改：润色两处过渡，去除 AI 味用词。"), result: "自审完成"},

		// 阶段 review：run_subagent 审稿（require: run_subagent）+ 自查重读 + 3 处修复 + 字数复查
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(ch)},
		{tool: "read", args: fmt.Sprintf(`{"path":"chapters/%03d.md"}`, ch), result: chapterBody[0] + chapterBody[1]},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：调整对话节奏，补充情绪铺垫。"), result: "已修复问题 1"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：前文伏笔在此回收，强化悬念。"), result: "已修复问题 2"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "修改：删减冗余描写，收紧节奏。"), result: "已修复问题 3"},
		{tool: "get_chapter_list", args: `{}`, result: fmt.Sprintf(`{"check_chapter":%d,"word_count":2980,"word_count_ok":true,"min_words":2500,"max_words":4000}`, ch)},
		{tool: "set_phase", args: `{"phase":"maintain"}`, result: `{"success":true,"phase":"maintain"}`},

		// ── maintain：7 项状态查询 + 搜索防遗忘 + 6 类更新 + goink.md（require 13 项）──
		{tool: "get_characters", args: `{}`, result: `{"characters":[{"id":1,"name":"陈昊","desc":"主角","status":"突破金丹"}]}`},
		{tool: "get_timeline", args: `{}`, result: `{"foreshadow":[{"id":5,"title":"玉佩来历","target_chapter":8,"status":"pending"}]}`},
		{tool: "get_story_arcs", args: `{}`, result: `{"arcs":[{"id":1,"name":"登天之路","nodes_done":3,"nodes_total":10}]}`},
		{tool: "get_reader_perspective", args: `{}`, result: `{"known":["陈昊身怀异火"],"suspense":["玉佩来历"],"misconception":[]}`},
		{tool: "get_scenes", args: fmt.Sprintf(`{"chapter_id":%d}`, ch), result: `{"scenes":[{"id":10,"title":"秘境初探","summary":"陈昊入秘境","word_count":3000}]}`},
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
		{tool: "set_phase", args: `{"phase":"prepare"}`, result: `{"success":true,"phase":"prepare"}`},
	}
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
		fmt.Fprintf(&b, `{"num":%d,"title":"%s","word_count":3000}`, i, chapterTitle(i))
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
	b.WriteString(`,"title":"第二章","word_count":3000},"recent_chapters":[`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"num":%d,"title":"章","word_count":3000}`, i)
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
	sub := append(append([]map[string]any{}, history...), cur...)
	sub = append(sub,
		map[string]any{"role": "system", "content": "你是审稿人（review agent），基于以下上下文审阅本章。\n\n" + novelState(turn)},
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
		hit, miss := cache.Step(sub)
		results = append(results, [2]int64{hit, miss})
		sub = append(sub,
			asstToolCall(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, sp.args),
			toolMsg(fmt.Sprintf("sub_t%d_p%d", turn, i), sp.tool, sp.result),
		)
	}
	// 子 agent 最终回复（审读报告）
	hit, miss := cache.Step(sub)
	results = append(results, [2]int64{hit, miss})
	sub = append(sub, asstText(reviewReport(turn)))

	return results
}

// buildGate 门禁创作 5 轮。每轮 20 次工具调用 + 1 次最终回复。
// 两种协议的差异与短问答相同：NS 是否进入历史。
// NS 在当轮紧跟 user（旧实现注入在 loadAPIMessages 结果之后、工具循环之前），
// 当轮内保持；唯一差异是历史是否保留 NS。
func buildGate(mode string, cache *TokenCache) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

	// 首轮：init 开书流程（建世界观/角色/弧线 + 写总纲 + 建卷），
	// 之后才进入 prepare 循环。NS 首次注入跟随首轮 user。
	cur := []map[string]any{userMsg("请开始创作：这是一本仙侠小说《登天之路》。")}
	cur = append(cur, sysMsg(novelState(0)))
	for i, p := range initScript() {
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})
		cur = append(cur,
			asstToolCall(fmt.Sprintf("call_init_p%d", i), p.tool, p.args),
			toolMsg(fmt.Sprintf("call_init_p%d", i), p.tool, p.result),
		)
	}
	cur = append(cur, asstText("开书完成：世界观、角色、总纲、第一卷弧线已建立，进入第一章创作。"))
	req := append(append([]map[string]any{}, history...), cur...)
	hit, miss := cache.Step(req)
	results = append(results, [2]int64{hit, miss})
	if mode == "now" {
		history = append(history, cur...)
	} else {
		legacyCur := append([]map[string]any{}, cur[0])
		legacyCur = append(legacyCur, cur[2:]...) // 跳过 cur[1]（NS）
		history = append(history, legacyCur...)
	}

	for turn := 1; turn <= 5; turn++ {
		// 当轮动态消息：user + NS（两种协议都紧跟 user）
		cur := []map[string]any{userMsg(fmt.Sprintf("请创作第 %d 章，继续推进剧情。", turn+1))}
		cur = append(cur, sysMsg(novelState(turn)))

		// 逐个工具调用：每次先发当前已累积上下文，产出下一条 tool_call 后再追加
		plays := gateScript(turn)
		for i, p := range plays {
			req := append(append([]map[string]any{}, history...), cur...)
			hit, miss := cache.Step(req)
			results = append(results, [2]int64{hit, miss})
			// 模型产出的 tool_call + 执行结果，进入当轮累积
			cur = append(cur, asstToolCall(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.args))
			// run_subagent：模拟子 agent 内部请求序列（与 RunSubAgent fork 完整主历史一致）
			if p.tool == "run_subagent" {
				subResults := simulateSubagent(cache, history, cur, turn)
				results = append(results, subResults...)
			}
			cur = append(cur, toolMsg(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.result))
		}
		// 最后一轮工具后模型产出最终正文
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		// 更新历史：now 含 NS（落库）；legacy 不含 NS（真实修复前：NS 不落库，
		// user 与工具结果正常落库，请求时 NS 临时拼在尾部）
		if mode == "now" {
			history = append(history, cur...)
		} else {
			legacyCur := append([]map[string]any{}, cur[0])
			legacyCur = append(legacyCur, cur[2:]...) // 跳过 cur[1]（NS）
			history = append(history, legacyCur...)
		}
	}
	return results
}

// ---- 运行 ----

func main() {
	mode := "compare"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode != "now" && mode != "legacy" && mode != "compare" {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/cacheprobe [now|legacy|compare]\n")
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.DiscardHandler))
	initTools()

	if mode == "compare" {
		runCompare()
		return
	}

	label := map[string]string{"now": "修复后：NS 落库", "legacy": "修复前：NS 不落库"}[mode]
	fmt.Println("================================================================")
	fmt.Println(" cacheprobe：消息级缓存命中模拟（tiktoken 精确计数）")
	fmt.Println(" 协议: " + label)
	fmt.Println("================================================================")

	runScenario("短问答 5 轮", mode, buildShortQA)
	runScenario("门禁创作 5 轮", mode, buildGate)
}

// runCompare 一次跑完两种协议并输出汇总对照。
func runCompare() {
	fmt.Println("================================================================")
	fmt.Println(" cacheprobe：缓存命中对照（消息级前缀模拟，tiktoken 精确计数）")
	fmt.Println("  对比：修复前（NS 不落库） vs 修复后（NS 落库）")
	fmt.Println("================================================================")

	type scenario struct {
		name string
		fn   func(string, *TokenCache) [][2]int64
	}
	for _, s := range []scenario{
		{"短问答 5 轮", buildShortQA},
		{"门禁创作 5 轮", buildGate},
	} {
		nowCache := NewTokenCache()
		legacyCache := NewTokenCache()
		nowR := s.fn("now", nowCache)
		legR := s.fn("legacy", legacyCache)

		var nowHit, nowMiss, legHit, legMiss int64
		for _, pr := range nowR {
			nowHit += pr[0]
			nowMiss += pr[1]
		}
		for _, pr := range legR {
			legHit += pr[0]
			legMiss += pr[1]
		}
		fmt.Printf("\n=== %s ===\n", s.name)
		fmt.Printf("  修复前  hit=%12d miss=%12d 命中率=%5.1f%%\n", legHit, legMiss, pct(legHit, legMiss))
		fmt.Printf("  修复后  hit=%12d miss=%12d 命中率=%5.1f%%\n", nowHit, nowMiss, pct(nowHit, nowMiss))
		missSave := float64(legMiss-nowMiss) / float64(legMiss) * 100
		fmt.Printf("  miss 降幅 = %.1f%%（未命中的 token 直接按全价计费，此项即真实成本节约）\n", missSave)
	}
}

func pct(hit, miss int64) float64 {
	if hit+miss == 0 {
		return 0
	}
	return 100 * float64(hit) / float64(hit+miss)
}

func runScenario(name, mode string, fn func(string, *TokenCache) [][2]int64) {
	cache := NewTokenCache()
	fmt.Printf("\n=== %s ===\n", name)
	fmt.Printf("%4s | %12s | %12s | %8s\n", "调用", "hit", "miss", "累计命中率")
	fmt.Println("------|--------------|--------------|----------")
	totHit, totMiss := int64(0), int64(0)
	for i, pr := range fn(mode, cache) {
		totHit += pr[0]
		totMiss += pr[1]
		rate := 0.0
		if totHit+totMiss > 0 {
			rate = 100 * float64(totHit) / float64(totHit+totMiss)
		}
		fmt.Printf("%4d | %12d | %12d | %7.1f%%\n", i+1, pr[0], pr[1], rate)
	}
	fmt.Printf("------|--------------|--------------|----------\n")
	fmt.Printf("累计  | hit=%d miss=%d 命中率=%.1f%%\n", totHit, totMiss,
		100*float64(totHit)/float64(totHit+totMiss))
	_ = mode
}
