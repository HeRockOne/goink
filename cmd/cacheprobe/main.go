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
main-tech-golden-finger-design 金手指；main-tech-chapter-title-hooks 标题钩子；main-tech-book-completion 完本清单；
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
	chapterBody = []string{
		body[0:1000], body[1000:2000], body[2000:3000],
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

// gateScript 一轮创作的工具剧本，严格按门禁配置 single 模式的
// prepare → outline → write → review → maintain 完整流程 + main-core-writing-kernel 调度。
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
		readSkill("main-tech-common-sense-logic"),
		readSkill("main-tech-genre-templates"),
		{tool: "set_phase", args: `{"phase":"outline"}`, result: `{"success":true,"phase":"outline"}`},

		// ── outline：加载 5 个大纲技能 + 写大纲（require: edit）──
		readSkill("main-tech-emotion-injection"),
		readSkill("main-tech-chapter-hook-enhanced"),
		readSkill("main-tech-maliang-method"),
		readSkill("main-tech-dialogue-subtext"),
		readSkill("main-tech-chapter-title-hooks"),
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), outlineText(ch)), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "edit", args: editArgs(fmt.Sprintf("outlines/%03d.md", ch), "## 关键事件\n1. 陈昊进入秘境\n2. 遭遇仇敌\n3. 突破金丹\n\n## 章末钩子\n玉佩发出异光"), result: fmt.Sprintf("写入 outlines/%03d.md", ch)},
		{tool: "set_phase", args: `{"phase":"write"}`, result: `{"success":true,"phase":"write"}`},

		// ── write：加载 4 个正文技能 + 写正文 3000 字（require: edit, get_chapter_list）+ 记录物品 ──
		readSkill("main-tech-show-dont-tell"),
		readSkill("main-tech-info-density"),
		readSkill("main-tech-pov-purity"),
		readSkill("main-tech-anti-ai-writing"),
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[0]), result: "写入 1000 字，当前 1000/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[1]), result: "写入 1000 字，当前 2000/3000"},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), chapterBody[2]), result: "写入 1000 字，当前 3000/3000"},
		{tool: "create_item_occurrence", args: `{"item_id":3,"chapter_id":` + fmt.Sprintf("%d", ch) + `,"action":"陈昊获得玉佩"}`, result: "已记录"},
		{tool: "set_phase", args: `{"phase":"review"}`, result: `{"success":true,"phase":"review"}`},

		// ── review：run_subagent 审稿（require: run_subagent）──
		{tool: "run_subagent", args: `{"agent_type":"review"}`, result: reviewReport(ch)},
		{tool: "edit", args: editArgs(fmt.Sprintf("chapters/%03d.md", ch), "（根据审稿意见修复：段 3 节奏放缓，补现场描写 120 字）"), result: "已修复"},
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
		{tool: "update_reader_perspective_entry", args: `{"entry_id":7,"content":"玉佩与陈昊身世有关","type":"suspense"}`, result: "已更新"},
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

// buildGate 门禁创作 5 轮。每轮 20 次工具调用 + 1 次最终回复。
// 两种协议的差异与短问答相同：NS 是否进入历史。
// NS 在当轮紧跟 user（旧实现注入在 loadAPIMessages 结果之后、工具循环之前），
// 当轮内保持；唯一差异是历史是否保留 NS。
func buildGate(mode string, cache *TokenCache) [][2]int64 {
	results := [][2]int64{}
	history := append([]map[string]any{}, fixedSystem()...)

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
			cur = append(cur,
				asstToolCall(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.args),
				toolMsg(fmt.Sprintf("call_t%d_p%d", turn, i), p.tool, p.result),
			)
		}
		// 最后一轮工具后模型产出最终正文
		cur = append(cur, asstText(finalAssistant(turn)))
		req := append(append([]map[string]any{}, history...), cur...)
		hit, miss := cache.Step(req)
		results = append(results, [2]int64{hit, miss})

		// 更新历史：now 含 NS；legacy 不含 NS（NS 未落库）
		if mode == "now" {
			history = append(history, cur...)
		} else {
			history = append(history, cur[1:]...)
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
