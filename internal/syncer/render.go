package syncer

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderMarkdown(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	if text, ok := value.(string); ok {
		return normalizeMarkdown(text), nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var node pmNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("decode rich-text document: %w", err)
	}
	return normalizeMarkdown(renderNode(node, 0)), nil
}

type mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}
type pmNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Content []pmNode       `json:"content"`
	Marks   []mark         `json:"marks"`
}

func renderNode(n pmNode, depth int) string {
	children := func() string {
		var b strings.Builder
		for _, c := range n.Content {
			b.WriteString(renderNode(c, depth+1))
		}
		return b.String()
	}
	switch n.Type {
	case "doc":
		return children()
	case "text":
		t := n.Text
		for _, m := range n.Marks {
			switch m.Type {
			case "bold", "strong":
				t = "**" + t + "**"
			case "italic", "em":
				t = "_" + t + "_"
			case "code":
				t = "`" + t + "`"
			case "link":
				if href, _ := m.Attrs["href"].(string); href != "" {
					t = "[" + t + "](" + href + ")"
				}
			}
		}
		return t
	case "paragraph":
		return children() + "\n\n"
	case "heading":
		level := 1
		if v, ok := n.Attrs["level"].(float64); ok && v > 0 {
			level = int(v)
		}
		return strings.Repeat("#", level) + " " + strings.TrimSpace(children()) + "\n\n"
	case "bulletList", "orderedList":
		return children() + "\n"
	case "listItem":
		return strings.Repeat("  ", max(depth-3, 0)) + "- " + strings.TrimSpace(children()) + "\n"
	case "blockquote":
		return "> " + strings.ReplaceAll(strings.TrimSpace(children()), "\n", "\n> ") + "\n\n"
	case "codeBlock":
		return "```\n" + strings.TrimSuffix(children(), "\n") + "\n```\n\n"
	case "hardBreak":
		return "\n"
	case "horizontalRule":
		return "---\n\n"
	case "image":
		if src, _ := n.Attrs["src"].(string); src != "" {
			return "![](" + src + ")\n\n"
		}
		return ""
	default:
		return children()
	}
}

func normalizeMarkdown(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) + "\n"
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
