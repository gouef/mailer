package mailer

import (
	"mime"
	"net/url"
	"path/filepath"
	"strings"
)

func getFileExt(name string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return ext
}

func mediaType(baseType string, params map[string]string) string {
	if len(params) == 0 {
		return baseType
	}

	if value := mime.FormatMediaType(baseType, params); value != "" {
		return value
	}

	return baseType
}

func formatContentDisposition(disposition string, filename string) string {
	trimmed := strings.TrimSpace(filename)
	if trimmed == "" {
		return disposition
	}

	if isASCII(trimmed) {
		if value := mime.FormatMediaType(disposition, map[string]string{"filename": trimmed}); value != "" {
			return value
		}
		return disposition
	}

	params := map[string]string{
		"filename":  sanitizeASCIIFilename(trimmed),
		"filename*": "utf-8''" + url.PathEscape(trimmed),
	}

	if value := mime.FormatMediaType(disposition, params); value != "" {
		return value
	}

	return disposition
}

func isASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > 127 {
			return false
		}
	}
	return true
}

func sanitizeASCIIFilename(value string) string {
	b := strings.Builder{}
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] <= 127 && value[i] >= 32 {
			b.WriteByte(value[i])
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}
