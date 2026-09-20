package changelog

import (
	"embed"
	"regexp"
	"strings"
)

//go:embed notes/*.md
var notesFS embed.FS

// Notes 持有当前版本的中英文更新日志正文。
// Version 来自调用方（通常是 wails.json 中的版本号），
// BodyZh / BodyEn 是文件内 `## 中文` / `## English` 标题之后的 markdown 正文。
type Notes struct {
	Version string `json:"version"`
	BodyZh  string `json:"body_zh"`
	BodyEn  string `json:"body_en"`
}

// sectionHeader 匹配 `## 中文` / `## English` 两个二级标题（独占一行，允许末尾空白）。
var sectionHeader = regexp.MustCompile(`(?m)^##[ \t]+(中文|English)[ \t]*$`)

// Get 读取 notes/v<version>.md，按 `## 中文` / `## English` 切段返回。
// 文件不存在或两段都为空时返回 (Notes{}, false)。
func Get(version string) (Notes, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return Notes{}, false
	}
	filename := "notes/v" + version + ".md"
	data, err := notesFS.ReadFile(filename)
	if err != nil {
		return Notes{}, false
	}
	zh, en := splitBilingual(string(data))
	if zh == "" && en == "" {
		return Notes{}, false
	}
	return Notes{Version: version, BodyZh: zh, BodyEn: en}, true
}

// splitBilingual 按 `## 中文` / `## English` 切分输入内容。
// 缺一段则该段返回空字符串。
func splitBilingual(content string) (zh, en string) {
	// 先归一化行尾：notes/*.md 在 Windows 检出（历史上曾带 CRLF）时，
	// `^## 头部[ \t]*$` 无法匹配到 \r 之前，会让两段都切不出来。
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	matches := sectionHeader.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return "", ""
	}
	for i, m := range matches {
		bodyStart := m[1]
		bodyEnd := len(content)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		body := strings.TrimSpace(content[bodyStart:bodyEnd])
		label := content[m[2]:m[3]]
		switch label {
		case "中文":
			zh = body
		case "English":
			en = body
		}
	}
	return zh, en
}
