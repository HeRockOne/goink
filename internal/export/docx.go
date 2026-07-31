package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"strings"

	"novel/internal/novel"
)

func exportDocx(n *novel.Novel, chapters []ChapterWithContent) ([]byte, string, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// [Content_Types].xml
	writeZip(w, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`)

	// _rels/.rels
	writeZip(w, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	// word/_rels/document.xml.rels
	writeZip(w, "word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`)

	// word/styles.xml
	writeZip(w, "word/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:pPr><w:spacing w:after="200" w:line="360" w:lineRule="auto"/></w:pPr>
    <w:rPr><w:sz w:val="24"/><w:rFonts w:ascii="宋体" w:eastAsia="宋体"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/>
    <w:pPr><w:jc w:val="center"/><w:spacing w:before="600" w:after="400"/></w:pPr>
    <w:rPr><w:sz w:val="44"/><w:b/><w:rFonts w:ascii="黑体" w:eastAsia="黑体"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:pPr><w:spacing w:before="400" w:after="200"/></w:pPr>
    <w:rPr><w:sz w:val="32"/><w:b/><w:rFonts w:ascii="黑体" w:eastAsia="黑体"/></w:rPr>
  </w:style>
</w:styles>`)

	// word/document.xml
	var docParts []string
	docParts = append(docParts, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>`)

	// 小说标题
	title := n.Title
	if title == "" {
		title = "未命名"
	}
	docParts = append(docParts, fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`, html.EscapeString(title)))

	for i, ch := range chapters {
		chTitle := ch.Chapter.Title
		if chTitle == "" {
			chTitle = fmt.Sprintf("第 %d 章", i+1)
		}
		docParts = append(docParts, fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`, html.EscapeString(chTitle)))

		// 章节内容按段落拆分
		paras := strings.Split(ch.Content, "\n")
		for _, para := range paras {
			para = strings.TrimSpace(para)
			if para == "" {
				continue
			}
			docParts = append(docParts, fmt.Sprintf(`<w:p><w:r><w:t>%s</w:t></w:r></w:p>`, html.EscapeString(para)))
		}
	}

	docParts = append(docParts, `</w:body></w:document>`)
	writeZip(w, "word/document.xml", strings.Join(docParts, "\n"))

	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("docx: zip close: %w", err)
	}

	filename := safeFilename(title) + ".docx"
	return buf.Bytes(), filename, nil
}

func writeZip(w *zip.Writer, name, content string) {
	f, _ := w.Create(name)
	f.Write([]byte(content))
}