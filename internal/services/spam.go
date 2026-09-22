package services

import "strings"

type SpamChecker struct {
	BlockedKeywords []string
}

var cjkRanges = [][2]rune{
	{0x4E00, 0x9FFF}, // CJK Unified Ideographs
	{0x3400, 0x4DBF}, // CJK Extension A
	{0x20000, 0x2A6DF},
	{0xF900, 0xFAFF}, // CJK Compatibility Ideographs
	{0x3040, 0x30FF}, // Hiragana / Katakana
	{0xAC00, 0xD7AF}, // Hangul
}

func hasCJK(s string) bool {
	for _, r := range s {
		for _, rng := range cjkRanges {
			if r >= rng[0] && r <= rng[1] {
				return true
			}
		}
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isReadable(s string) bool {
	if s == "" {
		return false
	}
	if hasCJK(s) {
		return true
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func (s SpamChecker) Check(data map[string]string) (bool, string) {
	if len(data) == 0 {
		return true, "empty submission"
	}
	for k, v := range data {
		if strings.TrimSpace(v) != "" {
			_ = k
			goto NEXT
		}
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

	// 内容启发式：对未开启验证码的表单也有兜底，仅标记垃圾不发邮件
	nonEmpty := 0
	allDigits := true
	anyReadable := false
	linkCount := 0
	for _, v := range data {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		nonEmpty++
		if !isAllDigits(v) {
			allDigits = false
		}
		if isReadable(v) {
			anyReadable = true
		}
		lower := strings.ToLower(v)
		linkCount += strings.Count(lower, "http://")
		linkCount += strings.Count(lower, "https://")
	}
	if nonEmpty >= 2 && allDigits {
		return true, "numeric garbage"
	}
	if nonEmpty >= 1 && !anyReadable {
		return true, "unreadable content"
	}
	if linkCount >= 4 {
		return true, "too many links"
	}

	return false, ""
}
