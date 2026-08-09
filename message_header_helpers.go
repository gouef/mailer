package mailer

import "strings"

func isValidHeaderName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}

	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		if b == ':' || b == '\r' || b == '\n' {
			return false
		}
		if b < 33 || b > 126 {
			return false
		}
	}

	return true
}

func formatHeaderLine(name string, value string, maxLine int) string {
	prefix := name + ": "
	trimmed := sanitizeHeaderValue(value)
	if maxLine <= 0 || len(prefix)+len(trimmed) <= maxLine {
		return prefix + trimmed + "\r\n"
	}

	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return prefix + "\r\n"
	}

	var b strings.Builder
	b.WriteString(prefix)
	lineLen := len(prefix)

	for i, word := range words {
		sep := ""
		addLen := len(word)
		if i > 0 {
			sep = " "
			addLen++
		}

		if i > 0 && lineLen+addLen > maxLine && lineLen > 1 {
			b.WriteString("\r\n ")
			b.WriteString(word)
			lineLen = 1 + len(word)
			continue
		}

		b.WriteString(sep)
		b.WriteString(word)
		lineLen += addLen
	}

	b.WriteString("\r\n")
	return b.String()
}

func sanitizeHeaderValue(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
