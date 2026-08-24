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
	totalRe   = regexp.MustCompile(`总分[:：]\s*([\d.]+)\s*/\s*10`)
	verdictRe = regexp.MustCompile(`总分[:：]\s*[\d.]+\s*/\s*10\s*[（(]\s*(通过|需修改|不通过)\s*[）)]`)
	fatalRe   = regexp.MustCompile(`致命问题[:：]?\s*(\d+)\s*项?`)
	dimRes    = map[string]*regexp.Regexp{
		"structure": regexp.MustCompile(`故事结构\s*[:：]?\s*([\d.]+)\s*/\s*10`),
		"character": regexp.MustCompile(`角色深度\s*[:：]?\s*([\d.]+)\s*/\s*10`),
		"pacing":    regexp.MustCompile(`节奏与爽点\s*[:：]?\s*([\d.]+)\s*/\s*10`),
		"prose":     regexp.MustCompile(`散文工艺\s*[:：]?\s*([\d.]+)\s*/\s*10`),
		"scene":     regexp.MustCompile(`场景工程\s*[:：]?\s*([\d.]+)\s*/\s*10`),
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
	switch {
	case strings.Contains(report, "不通过"):
		r.Verdict = VerdictFail
	case verdictRe.MatchString(report):
		m := verdictRe.FindStringSubmatch(report)
		switch m[1] {
		case "通过":
			r.Verdict = VerdictPass
		case "需修改":
			r.Verdict = VerdictRevise
		}
	default:
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
