package services

import "strings"

type SpamChecker struct {
	BlockedKeywords []string
}

func (s SpamChecker) Check(data map[string]string) (bool, string) {
	if len(data) == 0 {
		return true, "empty submission"
	}
	for k, v := range data {
		if strings.TrimSpace(v) != "" {
			goto NEXT
		}
		_ = k
	}
	return true, "all fields empty"

NEXT:
	text := ""
	for k, v := range data {
		text += strings.ToLower(k + " " + v + "\n")
	}
	for _, kw := range s.BlockedKeywords {
		if kw == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(kw)) {
			return true, "blocked keyword: " + kw
		}
	}
	return false, ""
}
