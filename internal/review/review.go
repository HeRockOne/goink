package review

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ReviewRecord 是一次审稿的持久化记录。
// 由代码在 RunSubAgent 返回时确定性写入，不依赖 AI 自觉调用。
type ReviewRecord struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	NovelID      int64     `gorm:"column:novel_id;index" json:"novel_id"`
	SessionID    string    `gorm:"column:session_id;index;size:64" json:"session_id"`
	ChapterStart int       `gorm:"column:chapter_start" json:"chapter_start"` // 审读章节范围，从 instruction best-effort 解析，0=未知
	ChapterEnd   int       `gorm:"column:chapter_end" json:"chapter_end"`
	TotalScore   float64   `gorm:"column:total_score" json:"total_score"` // -1 = 解析失败
	Verdict      string    `gorm:"column:verdict;size:16" json:"verdict"` // pass / revise / fail / unknown
	FatalCount   int       `gorm:"column:fatal_count" json:"fatal_count"` // 致命问题数，-1 = 解析失败
	DimStructure float64   `gorm:"column:dim_structure" json:"dim_structure"`
	DimCharacter float64   `gorm:"column:dim_character" json:"dim_character"`
	DimPacing    float64   `gorm:"column:dim_pacing" json:"dim_pacing"`
	DimProse     float64   `gorm:"column:dim_prose" json:"dim_prose"`
	DimScene     float64   `gorm:"column:dim_scene" json:"dim_scene"`
	Instruction  string    `gorm:"column:instruction;type:text" json:"instruction"`
	Report       string    `gorm:"column:report;type:text" json:"report"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ReviewRecord) TableName() string { return "review_records" }

const (
	VerdictPass    = "pass"
	VerdictRevise  = "revise"
	VerdictFail    = "fail"
	VerdictUnknown = "unknown"
)

var (
	// totalRe 匹配总分：8.5/10 或 | 总分 | 8.5/10 |（markdown 表格）或 **总分：8.5/10**
	totalRe = regexp.MustCompile(`(?:\*\*)?总分[:：]?\s*(?:\*\*)?\s*[\|]?\s*([\d.]+)\s*/\s*10`)
	// verdictRe 匹配总分：8.5/10（需修改）或 | 8.5/10（需修改） |
	verdictRe = regexp.MustCompile(`(?:\*\*)?总分[:：]?\s*(?:\*\*)?\s*[\|]?\s*[\d.]+\s*/\s*10\s*[（(]\s*(通过|需修改|不通过)\s*[）)]`)
	// fatalRe 匹配 致命问题：0项 或 **致命问题**：0项 或 | 致命问题 | 0项 |
	fatalRe = regexp.MustCompile(`(?:\*\*)?致命问题(?:\*\*)?[:：]?\s*[\|]?\s*(\d+)\s*项?`)
	// dimRes 匹配 故事结构 9/10 或 故事结构：9/10 或 | 故事结构 | 9/10 |（markdown 表格行）
	dimRes = map[string]*regexp.Regexp{
		"structure": regexp.MustCompile(`故事结构\s*[:：]?\s*[\|]?\s*([\d.]+)\s*/\s*10`),
		"character": regexp.MustCompile(`角色深度\s*[:：]?\s*[\|]?\s*([\d.]+)\s*/\s*10`),
		"pacing":    regexp.MustCompile(`节奏与爽点\s*[:：]?\s*[\|]?\s*([\d.]+)\s*/\s*10`),
		"prose":     regexp.MustCompile(`散文工艺\s*[:：]?\s*[\|]?\s*([\d.]+)\s*/\s*10`),
		"scene":     regexp.MustCompile(`场景工程\s*[:：]?\s*[\|]?\s*([\d.]+)\s*/\s*10`),
	}
	rangeRe  = regexp.MustCompile(`第\s*(\d+)\s*章\s*[-–~至到]{1,2}\s*第?\s*(\d+)\s*章`)
	singleRe = regexp.MustCompile(`第\s*(\d+)\s*章`)
)

// ParseReport 从审稿报告原文 + 任务指令中 best-effort 提取结构化字段。
// 解析不出时对应字段取零值/unknown，报告原文始终全量保留。
func ParseReport(report, instruction string) *ReviewRecord {
	r := &ReviewRecord{Report: report, Instruction: instruction}
	r.ChapterStart, r.ChapterEnd = parseChapterRange(instruction + "\n" + report)

	if m := totalRe.FindStringSubmatch(report); m != nil {
		r.TotalScore, _ = strconv.ParseFloat(m[1], 64)
	} else {
		r.TotalScore = -1
	}
	// 先尝试结构化结论行（总分：X.X/10（结论）），再 fallback 到纯文本匹配
	if verdictRe.MatchString(report) {
		m := verdictRe.FindStringSubmatch(report)
		switch m[1] {
		case "通过":
			r.Verdict = VerdictPass
		case "需修改":
			r.Verdict = VerdictRevise
		case "不通过":
			r.Verdict = VerdictFail
		}
	} else if strings.Contains(report, "不通过") {
		r.Verdict = VerdictFail
	} else {
		r.Verdict = VerdictUnknown
	}
	if m := fatalRe.FindStringSubmatch(report); m != nil {
		r.FatalCount, _ = strconv.Atoi(m[1])
	} else {
		r.FatalCount = -1
	}
	for key, re := range dimRes {
		var v float64
		if m := re.FindStringSubmatch(report); m != nil {
			v, _ = strconv.ParseFloat(m[1], 64)
		} else {
			v = -1
		}
		switch key {
		case "structure":
			r.DimStructure = v
		case "character":
			r.DimCharacter = v
		case "pacing":
			r.DimPacing = v
		case "prose":
			r.DimProse = v
		case "scene":
			r.DimScene = v
		}
	}
	return r
}

// parseChapterRange 从文本提取章节范围。支持"第3-5章""第3章到第5章"，单个"第N章"起止相同。
func parseChapterRange(text string) (int, int) {
	if m := rangeRe.FindStringSubmatch(text); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > b {
			a, b = b, a
		}
		return a, b
	}
	if m := singleRe.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, n
	}
	return 0, 0
}

// SaveRecord 落库一条审稿记录，失败只告警不影响主流程。
func SaveRecord(db *gorm.DB, rec *ReviewRecord, log *slog.Logger) {
	if db == nil || rec == nil {
		return
	}
	if err := db.Create(rec).Error; err != nil {
		if log != nil {
			log.Warn("审稿记录落库失败", "err", err, "novel_id", rec.NovelID)
		}
	}
}

// 五维权重：与 sub-tech-review-standards.md 的维度权重表一一对应，改动需两处同步。
const (
	WeightStructure = 0.30
	WeightCharacter = 0.25
	WeightPacing    = 0.20
	WeightProse     = 0.15
	WeightScene     = 0.10
)

// ComputeTotalScore 按固定权重计算加权总分（保留两位小数）。
// 总分由代码计算是唯一真相，模型只提交各维度分数。
func ComputeTotalScore(structure, character, pacing, prose, scene float64) float64 {
	total := structure*WeightStructure + character*WeightCharacter +
		pacing*WeightPacing + prose*WeightProse + scene*WeightScene
	return float64(int(total*100+0.5)) / 100
}

// DeriveVerdict 按审稿标准推导结论：
// 含致命问题或总分<7.0 → fail；总分≥9.0 且无致命 → pass；其余 revise。
func DeriveVerdict(total float64, fatalCount int) string {
	if fatalCount > 0 || total < 7.0 {
		return VerdictFail
	}
	if total >= 9.0 {
		return VerdictPass
	}
	return VerdictRevise
}
