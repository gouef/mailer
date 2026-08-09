package mailer

import (
	"bytes"
	"mime"
	"mime/quotedprintable"
	"strings"
)

func encodeTextBody(body string) (string, []byte, error) {
	if isBodySevenBitSafe(body) {
		return "7bit", []byte(body), nil
	}

	buf := bytes.NewBuffer(nil)
	writer := quotedprintable.NewWriter(buf)
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return "", nil, err
	}
	if err := writer.Close(); err != nil {
		return "", nil, err
	}

	return "quoted-printable", buf.Bytes(), nil
}

func isBodySevenBitSafe(body string) bool {
	lineLen := 0
	for i := 0; i < len(body); i++ {
		b := body[i]

		if b == '\r' {
			continue
		}

		if b == '\n' {
			lineLen = 0
			continue
		}

		if b > 127 {
			return false
		}

		lineLen++
		if lineLen > 998 {
			return false
		}
	}

	return true
}

func encodeHeaderValue(name string, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if !headerNeedsEncodedWord(name, trimmed) {
		return trimmed
	}

	return mime.BEncoding.Encode("utf-8", trimmed)
}

func headerNeedsEncodedWord(name string, value string) bool {
	if !hasNonASCII(value) {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "subject", "comments", "keywords", "organization":
		return true
	}

	return strings.HasPrefix(lower, "x-")
}

func hasNonASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > 127 {
			return true
		}
	}

	return false
}
