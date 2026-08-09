package mailer

import (
	"bytes"
	"errors"
	"mime/multipart"
	"strings"
	"testing"
)

func TestMessageHelpersCloneNilAndHeaderFormatting(t *testing.T) {
	var nilMessage *Message
	if nilMessage.Clone() != nil {
		t.Fatalf("expected nil clone for nil message")
	}

	if got := formatHeaderLine("X-Test", "   ", 78); got != "X-Test: \r\n" {
		t.Fatalf("unexpected blank header formatting: %q", got)
	}

	if got := formatHeaderLine("X-Test", "value", 0); got != "X-Test: value\r\n" {
		t.Fatalf("unexpected header formatting with disabled folding: %q", got)
	}

	longToken := strings.Repeat("a", 120)
	formatted := formatHeaderLine("X-Test", longToken, 20)
	if !strings.HasPrefix(formatted, "X-Test: ") {
		t.Fatalf("expected header prefix, got: %q", formatted)
	}
	if strings.HasPrefix(formatted, "X-Test: \r\n") {
		t.Fatalf("first token must not be folded onto empty line: %q", formatted)
	}
}

func TestMessageHelpersSplitAttachmentsAndBodyBuilders(t *testing.T) {
	inline := NewInlineAttachment("logo.png", []byte("img"), "cid-1")
	regular := NewAttachment("doc.txt", []byte("doc"))
	m := &Message{Attachments: []Attachment{inline, regular}}
	inlineList, regularList := splitAttachments(m.Attachments)

	if len(inlineList) != 1 || len(regularList) != 1 {
		t.Fatalf("unexpected attachment split: inline=%d regular=%d", len(inlineList), len(regularList))
	}

	altBuf, boundary, err := buildAlternativePart("Příliš", "<p>html</p>")
	if err != nil {
		t.Fatalf("buildAlternativePart failed: %v", err)
	}
	altText := altBuf.String()
	if boundary == "" || !strings.Contains(altText, boundary) {
		t.Fatalf("expected multipart boundary in alternative body, got: %s", altText)
	}
	if !strings.Contains(altText, "Content-Type: text/plain; charset=UTF-8") || !strings.Contains(altText, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected plain and html parts in alternative body, got: %s", altText)
	}

	body, contentType, err := buildBodyEntity("", "<p>html</p>", false, []Attachment{inline})
	if err != nil {
		t.Fatalf("buildBodyEntity failed: %v", err)
	}
	if !strings.Contains(contentType, "multipart/related") {
		t.Fatalf("expected multipart/related content type, got: %s", contentType)
	}
	if !strings.Contains(string(body), "Content-Id: <cid-1>") {
		t.Fatalf("expected inline content id, got:\n%s", string(body))
	}

	body, contentType, err = buildBodyEntity("plain", "<p>html</p>", true, nil)
	if err != nil {
		t.Fatalf("buildBodyEntity alternative failed: %v", err)
	}
	if !strings.Contains(contentType, "multipart/alternative") || !strings.Contains(string(body), "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("expected multipart alternative body entity, got type=%q body=\n%s", contentType, string(body))
	}

	body, contentType, err = buildBodyEntity("plain", "", false, []Attachment{NewInlineAttachment("dot.bin", []byte("x"), "cid")})
	if err != nil {
		t.Fatalf("buildBodyEntity related plain failed: %v", err)
	}
	if !strings.Contains(contentType, "multipart/related") || !strings.Contains(string(body), "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("expected related plain body entity, got type=%q body=\n%s", contentType, string(body))
	}
}

func TestMessageHelpersAttachmentWriterAndBase64Wrapper(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(buf)
	attachment := Attachment{
		Name:        "data.bin",
		Data:        []byte(strings.Repeat("x", 90)),
		ContentType: "application/custom",
	}

	if err := writeAttachmentPart(writer, attachment); err != nil {
		t.Fatalf("writeAttachmentPart failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer close failed: %v", err)
	}

	text := buf.String()
	if !strings.Contains(text, "Content-Type: application/custom;") {
		t.Fatalf("expected explicit content type in attachment part, got:\n%s", text)
	}
	if !strings.Contains(text, "Content-Transfer-Encoding: base64") {
		t.Fatalf("expected base64 transfer encoding, got:\n%s", text)
	}
	if !strings.Contains(text, "\r\n\r\neHh4") || !strings.Contains(text, "\r\n") {
		t.Fatalf("expected base64 payload with line wrapping, got:\n%s", text)
	}

	lineBuf := bytes.NewBuffer(nil)
	lineWriter := newBase64LineWriter(lineBuf)
	if _, err := lineWriter.Write([]byte(strings.Repeat("a", 80))); err != nil {
		t.Fatalf("base64 line writer failed: %v", err)
	}
	if !strings.Contains(lineBuf.String(), "\r\n") {
		t.Fatalf("expected wrapped base64 line output, got: %q", lineBuf.String())
	}

	failingWriter := newBase64LineWriter(&errByteWriter{failAfter: 76})
	if _, err := failingWriter.Write([]byte(strings.Repeat("a", 80))); err == nil {
		t.Fatalf("expected base64 line writer error when continuation write fails")
	}

	inlineBuf := bytes.NewBuffer(nil)
	inlineWriter := multipart.NewWriter(inlineBuf)
	inlineAttachment := Attachment{Name: "logo.png", Data: []byte("img"), Inline: true, ContentID: "cid-inline"}
	if err := writeAttachmentPart(inlineWriter, inlineAttachment); err != nil {
		t.Fatalf("writeAttachmentPart inline failed: %v", err)
	}
	if err := inlineWriter.Close(); err != nil {
		t.Fatalf("inline writer close failed: %v", err)
	}
	inlineText := inlineBuf.String()
	if !strings.Contains(inlineText, "Content-Disposition: inline;") || !strings.Contains(inlineText, "Content-Id: <cid-inline>") {
		t.Fatalf("expected inline disposition and content-id, got:\n%s", inlineText)
	}

	fallbackBuf := bytes.NewBuffer(nil)
	fallbackWriter := multipart.NewWriter(fallbackBuf)
	fallbackAttachment := Attachment{Name: "archive.bin", Data: []byte("bin")}
	if err := writeAttachmentPart(fallbackWriter, fallbackAttachment); err != nil {
		t.Fatalf("writeAttachmentPart fallback failed: %v", err)
	}
	if err := fallbackWriter.Close(); err != nil {
		t.Fatalf("fallback writer close failed: %v", err)
	}
	if !strings.Contains(fallbackBuf.String(), "Content-Type: application/octet-stream;") {
		t.Fatalf("expected fallback application/octet-stream content type, got:\n%s", fallbackBuf.String())
	}
}

type errByteWriter struct {
	written   int
	failAfter int
}

func (w *errByteWriter) Write(p []byte) (int, error) {
	if w.written >= w.failAfter {
		return 0, errors.New("forced write error")
	}
	remaining := w.failAfter - w.written
	if remaining < len(p) {
		w.written += remaining
		return remaining, errors.New("forced write error")
	}
	w.written += len(p)
	return len(p), nil
}

func TestMessageHelpersEncodingUtilities(t *testing.T) {
	if got := mediaType("text/plain", nil); got != "text/plain" {
		t.Fatalf("expected plain base type for nil params, got %q", got)
	}

	if got := formatContentDisposition("attachment", ""); got != "attachment" {
		t.Fatalf("expected unchanged disposition for empty filename, got %q", got)
	}

	if got := formatContentDisposition("attachment", "ascii.txt"); !strings.Contains(got, "filename=ascii.txt") {
		t.Fatalf("expected ascii filename disposition, got %q", got)
	}

	if got := encodeHeaderValue("Subject", "   "); got != "" {
		t.Fatalf("expected blank encoded header value, got %q", got)
	}

	if got := relaxedBodyCanonicalization(""); got != "\r\n" {
		t.Fatalf("expected empty relaxed body canonicalization to end with CRLF, got %q", got)
	}

	if !strings.Contains(formatContentDisposition("attachment", "fakturá-číslo.pdf"), "filename*=") {
		t.Fatalf("expected encoded filename parameter")
	}

	if encoding, _, err := encodeTextBody(strings.Repeat("a", 999)); err != nil || encoding != "quoted-printable" {
		t.Fatalf("expected long line to force quoted-printable, got encoding=%q err=%v", encoding, err)
	}

	if isBodySevenBitSafe("line\r\n" + strings.Repeat("a", 999)) {
		t.Fatalf("expected overlong line with CRLF to be unsafe")
	}
}

func TestMessageHelpersMailboxAndMimeBranches(t *testing.T) {
	bad := Address{Email: "broken", Name: "Bad"}
	m := New()

	if _, err := m.SetFromMailbox(bad); err == nil {
		t.Fatalf("expected SetFromMailbox error")
	}
	if _, err := m.SetFromMailboxes([]Address{bad}); err == nil {
		t.Fatalf("expected SetFromMailboxes invalid mailbox error")
	}
	if _, err := m.AddFromAddress("invalid", "Bad"); err == nil {
		t.Fatalf("expected AddFromAddress error")
	}

	body, contentType, err := buildBodyEntity("plain", "", false, nil)
	if err != nil {
		t.Fatalf("buildBodyEntity plain failed: %v", err)
	}
	if contentType != "text/plain; charset=UTF-8" || string(body) != "plain" {
		t.Fatalf("unexpected plain body entity: type=%q body=%q", contentType, string(body))
	}

	body, contentType, err = buildBodyEntity("", "<b>html</b>", false, nil)
	if err != nil {
		t.Fatalf("buildBodyEntity html failed: %v", err)
	}
	if contentType != "text/html; charset=UTF-8" || string(body) != "<b>html</b>" {
		t.Fatalf("unexpected html body entity: type=%q body=%q", contentType, string(body))
	}

	headers, bodyBytes, contentType, err := m.buildMimeBody()
	if err != nil {
		t.Fatalf("buildMimeBody failed: %v", err)
	}
	if headers["Content-Transfer-Encoding"] != "7bit" || contentType != "text/plain; charset=UTF-8" || len(bodyBytes) != 0 {
		t.Fatalf("unexpected empty buildMimeBody result: headers=%v type=%q body=%q", headers, contentType, string(bodyBytes))
	}

	m = New().SetTextBody("body")
	m.AddAttachment("doc.txt", []byte("payload"))
	headers, bodyBytes, contentType, err = m.buildMimeBody()
	if err != nil {
		t.Fatalf("buildMimeBody with attachment failed: %v", err)
	}
	if len(headers) != 0 || !strings.Contains(contentType, "multipart/mixed") || !strings.Contains(string(bodyBytes), "Content-Disposition: attachment;") {
		t.Fatalf("unexpected attachment buildMimeBody result: headers=%v type=%q body=%s", headers, contentType, string(bodyBytes))
	}

	m = New().SetListUnsubscribe("mailto:test@example.com", "<https://example.com/u>")
	values := m.GetHeader("List-Unsubscribe")
	if len(values) != 1 || !strings.Contains(values[0], "<mailto:test@example.com>") || !strings.Contains(values[0], "<https://example.com/u>") {
		t.Fatalf("unexpected list-unsubscribe normalization: %v", values)
	}

	altBuf, _, err := buildAlternativePart("plain", "html")
	if err != nil {
		t.Fatalf("buildAlternativePart ascii failed: %v", err)
	}
	altText := altBuf.String()
	if strings.Count(altText, "Content-Transfer-Encoding: 7bit") != 2 {
		t.Fatalf("expected both alternative parts to use 7bit for ascii, got:\n%s", altText)
	}
}
