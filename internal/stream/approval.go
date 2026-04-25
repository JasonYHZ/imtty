package stream

import "strings"

type OutboundMessage struct {
	Text         string
	QuickReplies []string
}

func DetectQuickReplies(text string) []string {
	normalized := strings.ToLower(text)

	switch {
	case strings.Contains(normalized, "[y/n]"):
		return []string{"是", "否"}
	case strings.Contains(normalized, "(y/n)"):
		return []string{"是", "否"}
	case strings.Contains(normalized, " yes/no"):
		return []string{"是", "否"}
	case strings.Contains(normalized, "yes/no"):
		return []string{"是", "否"}
	case strings.Contains(normalized, "continue? y/n"):
		return []string{"是", "否"}
	default:
		return nil
	}
}
