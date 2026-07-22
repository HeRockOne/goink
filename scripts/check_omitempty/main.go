// check_omitempty 检查所有 Update*Input 结构体的字段是否都有 omitempty json tag。
// 用法：go run ./scripts/check_omitempty ./app/...
//
// 背景：PatchAndSave 用 json.Marshal/Unmarshal 合并字段，依赖 omitempty 跳过零值。
// 如果 Update*Input 的字段缺少 omitempty，json.Marshal 会序列化零值，导致 PatchAndSave
// 覆盖 entity 的原有字段为零值。
//
// 例外：在结构体上方加 //nolint:omitempty 注释可跳过检查（用于不走 PatchAndSave 的 Input）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 预编译正则，避免循环内反复编译
var (
	reUpdateInputStruct = regexp.MustCompile(`^type (Update\w+Input) struct \{`)
	reJsonTag           = regexp.MustCompile(`json:"([^"]*)"`)
	reStructName        = regexp.MustCompile(`type (Update\w+Input) struct`)
	reFieldName         = regexp.MustCompile(`^\s*(\w+)\s`)
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	missing := 0
	files := 0

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files++

		missing += checkFile(path, string(content))
		return nil
	})

	if missing > 0 {
		fmt.Printf("\n%d field(s) missing omitempty. All fields in Update*Input structs must have omitempty.\n", missing)
		fmt.Println("PatchAndSave relies on omitempty to skip zero-value fields.")
		fmt.Println("Add //nolint:omitempty comment above the struct to skip check.")
		os.Exit(1)
	}
	fmt.Printf("OK: all Update*Input structs have omitempty (checked %d files).\n", files)
}

// checkFile 检查单个文件，返回缺失 omitempty 的字段数
func checkFile(path, content string) int {
	missing := 0
	lines := strings.Split(content, "\n")

	i := 0
	for i < len(lines) {
		line := lines[i]
		// 匹配 type Update*Input struct {
		if !reUpdateInputStruct.MatchString(line) {
			i++
			continue
		}

		structName := extractStructName(line)
		// 检查上方注释是否有 nolint:omitempty
		if hasNolintComment(lines, i) {
			i++
			continue
		}

		// 遍历字段
		i++
		for i < len(lines) {
			fieldLine := strings.TrimSpace(lines[i])
			if fieldLine == "}" {
				break
			}
			if fieldLine == "" || strings.HasPrefix(fieldLine, "//") {
				i++
				continue
			}
			// 检查字段是否有 json tag 且含 omitempty
			if hasFieldWithoutOmitempty(fieldLine) {
				fieldName := extractFieldName(fieldLine)
				fmt.Printf("%s:%d: %s.%s missing omitempty in json tag\n",
					path, i+1, structName, fieldName)
				missing++
			}
			i++
		}
		i++
	}
	return missing
}

// hasNolintComment 检查结构体声明上方是否有 nolint:omitempty
func hasNolintComment(lines []string, typeLineIdx int) bool {
	for j := typeLineIdx - 1; j >= 0; j-- {
		line := strings.TrimSpace(lines[j])
		if line == "" {
			continue // 跳过空行
		}
		if strings.HasPrefix(line, "//") {
			if strings.Contains(line, "nolint:omitempty") {
				return true
			}
			continue // 继续往上找注释
		}
		break // 遇到非注释行，停止
	}
	return false
}

// hasFieldWithoutOmitempty 检查字段行是否有 json tag 但缺少 omitempty
func hasFieldWithoutOmitempty(line string) bool {
	// 匹配 json:"..." tag
	matches := reJsonTag.FindStringSubmatch(line)
	if matches == nil {
		return false // 无 json tag，不检查
	}
	tagValue := matches[1]
	items := strings.Split(tagValue, ",")
	for _, item := range items {
		if item == "omitempty" {
			return false
		}
	}
	return true
}

func extractStructName(line string) string {
	matches := reStructName.FindStringSubmatch(line)
	if matches != nil {
		return matches[1]
	}
	return ""
}

func extractFieldName(line string) string {
	matches := reFieldName.FindStringSubmatch(line)
	if matches != nil {
		return matches[1]
	}
	return ""
}
