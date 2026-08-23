package review

import (
	"strings"
	"testing"
)

const sampleReport = `【审稿评分报告】总分：8.5/10（需修改）

故事结构  9/10  权重30%  无明显扣分
角色深度  8/10  权重25%  配角动机略薄
节奏与爽点 8/10 权重20%  中段平缓
散文工艺  9/10  权重15%  个别病句
场景工程  8/10  权重10%  空间感稍弱

加权计算：9×0.30 + 8×0.25 + 8×0.20 + 9×0.15 + 8×0.10 = 8.5
致命问题：0 项

### 第二部分：问题清单
- 🟡 第3段：配角动机交代不足
`

func TestParseReport_Full(t *testing.T) {
	r := ParseReport(sampleReport, "审读第12章到第14章")
	if r.TotalScore != 8.5 {
		t.Errorf("TotalScore = %v, want 8.5", r.TotalScore)
	}
	if r.Verdict != VerdictRevise {
		t.Errorf("Verdict = %q, want %q", r.Verdict, VerdictRevise)
	}
	if r.FatalCount != 0 {
		t.Errorf("FatalCount = %v, want 0", r.FatalCount)
	}
	if r.DimStructure != 9 || r.DimCharacter != 8 || r.DimPacing != 8 || r.DimProse != 9 || r.DimScene != 8 {
		t.Errorf("dims = %v/%v/%v/%v/%v, want 9/8/8/9/8",
			r.DimStructure, r.DimCharacter, r.DimPacing, r.DimProse, r.DimScene)
	}
	if r.ChapterStart != 12 || r.ChapterEnd != 14 {
		t.Errorf("chapter range = %d-%d, want 12-14", r.ChapterStart, r.ChapterEnd)
	}
	if !strings.Contains(r.Report, "问题清单") {
		t.Error("report text should be preserved verbatim")
	}
}

func TestParseReport_Pass(t *testing.T) {
	report := strings.Replace(sampleReport, "总分：8.5/10（需修改）", "总分：9.2/10（通过）", 1)
	r := ParseReport(report, "审读第5章")
	if r.Verdict != VerdictPass {
		t.Errorf("Verdict = %q, want pass", r.Verdict)
	}
	if r.TotalScore != 9.2 {
		t.Errorf("TotalScore = %v, want 9.2", r.TotalScore)
	}
	if r.ChapterStart != 5 || r.ChapterEnd != 5 {
		t.Errorf("chapter range = %d-%d, want 5-5", r.ChapterStart, r.ChapterEnd)
	}
}

func TestParseReport_Garbage(t *testing.T) {
	r := ParseReport("完全不是审稿报告的文本", "")
	if r.Verdict != VerdictUnknown {
		t.Errorf("Verdict = %q, want unknown", r.Verdict)
	}
	if r.TotalScore != -1 || r.FatalCount != -1 {
		t.Errorf("unparseable fields should be -1, got score=%v fatal=%v", r.TotalScore, r.FatalCount)
	}
	if !strings.Contains(r.Report, "完全不是审稿报告") {
		t.Error("raw report should always be preserved")
	}
}

func TestParseReport_SingleChapterInReport(t *testing.T) {
	// instruction 为空时从报告正文提取章节号
	r := ParseReport("【审稿评分报告】第23章审稿意见……", "")
	if r.ChapterStart != 23 || r.ChapterEnd != 23 {
		t.Errorf("chapter range = %d-%d, want 23-23", r.ChapterStart, r.ChapterEnd)
	}
}
