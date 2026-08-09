package mailer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestToMIMEAutoMessageIDAndListUnsubscribe(t *testing.T) {
	m := New().
		SetFrom("Sender <sender@example.com>").
		AddTo("recipient@example.com").
		SetSubject("hello").
		SetTextBody("plain body").
		SetListUnsubscribe("https://example.com/unsubscribe", "mailto:unsubscribe@example.com").
		SetListUnsubscribePostOneClick()

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "Message-ID: <") {
		t.Fatalf("expected generated Message-ID header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "List-Unsubscribe: <https://example.com/unsubscribe>, <mailto:unsubscribe@example.com>") {
		t.Fatalf("expected List-Unsubscribe header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "List-Unsubscribe-Post: List-Unsubscribe=One-Click") {
		t.Fatalf("expected List-Unsubscribe-Post header, got:\n%s", mime)
	}
}

func TestToMIMEUsesCustomMessageIDHeader(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("hello").
		SetTextBody("body").
		SetHeader("Message-ID", "<custom-id@example.com>")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "Message-ID: <custom-id@example.com>") {
		t.Fatalf("expected custom Message-ID header, got:\n%s", mime)
	}
}

func TestMessageMetadataHelpers(t *testing.T) {
	m := New().
		SetFrom("Sender <sender@example.com>").
		AddTo("recipient@example.com").
		SetSubject("meta").
		SetTextBody("body").
		SetOrganization("Gouef").
		SetReadReceiptTo("receipt@example.com").
		SetPriority(1).
		SetMessageID("custom@id").
		SetReturnPath("bounce@example.com")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	checks := []string{
		"Organization: Gouef",
		"Disposition-Notification-To: receipt@example.com",
		"Return-Receipt-To: receipt@example.com",
		"X-Priority: 1",
		"Importance: High",
		"Priority: urgent",
		"Message-ID: <custom@id>",
		"Return-Path: <bounce@example.com>",
	}

	for _, item := range checks {
		if !strings.Contains(mime, item) {
			t.Fatalf("expected %q in mime:\n%s", item, mime)
		}
	}
}

func TestEnvelopeFromPrefersReturnPath(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		SetReturnPath("bounce@example.com")

	envelope, err := m.envelopeFrom()
	if err != nil {
		t.Fatalf("envelopeFrom failed: %v", err)
	}

	if envelope != "bounce@example.com" {
		t.Fatalf("unexpected envelope from: %s", envelope)
	}
}

func TestNamedMailboxHelpers(t *testing.T) {
	m := New()
	if _, err := m.SetFromAddress("sender@example.com", "Sender Name"); err != nil {
		t.Fatalf("SetFromAddress failed: %v", err)
	}
	if _, err := m.AddToAddress("recipient@example.com", "Recipient Name"); err != nil {
		t.Fatalf("AddToAddress failed: %v", err)
	}
	if _, err := m.AddCcAddress("cc@example.com", "CC Name"); err != nil {
		t.Fatalf("AddCcAddress failed: %v", err)
	}
	if _, err := m.AddBccAddress("bcc@example.com", "BCC Name"); err != nil {
		t.Fatalf("AddBccAddress failed: %v", err)
	}
	if _, err := m.AddReplyToAddress("reply@example.com", "Reply Name"); err != nil {
		t.Fatalf("AddReplyToAddress failed: %v", err)
	}

	m.SetSubject("named")
	m.SetTextBody("body")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	checks := []string{
		`From: "Sender Name" <sender@example.com>`,
		`To: "Recipient Name" <recipient@example.com>`,
		`Cc: "CC Name" <cc@example.com>`,
		`Reply-To: "Reply Name" <reply@example.com>`,
	}

	for _, item := range checks {
		if !strings.Contains(mime, item) {
			t.Fatalf("expected %q in mime:\n%s", item, mime)
		}
	}

	recipients := m.Recipients()
	foundBcc := false
	for _, recipient := range recipients {
		if strings.Contains(recipient, "bcc@example.com") {
			foundBcc = true
			break
		}
	}

	if !foundBcc {
		t.Fatalf("expected bcc recipient in recipients list")
	}
}

func TestNamedMailboxHelpersRejectInvalidAddress(t *testing.T) {
	m := New()

	if _, err := m.SetFromAddress("not-an-address", "Name"); err == nil {
		t.Fatalf("expected validation error")
	}

	if len(m.GetFrom()) != 0 {
		t.Fatalf("expected from to remain unchanged on invalid input")
	}
}

func TestToMIMEUsesMultipartRelatedForInlineAttachments(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("related").
		SetTextBody("plain").
		SetHtmlBody("<img src=\"cid:logo\" />")

	m.AddAttachment("file.txt", []byte("regular"))
	m.Attachments = append(m.Attachments, NewInlineAttachment("logo.png", []byte("img"), "logo"))

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "Content-Type: multipart/mixed;") {
		t.Fatalf("expected multipart/mixed root, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Content-Type: multipart/related;") {
		t.Fatalf("expected multipart/related part for inline content, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Content-Id: <logo>") {
		t.Fatalf("expected content-id for embedded file, got:\n%s", mime)
	}
}

func TestToMIMEFoldsLongHeaders(t *testing.T) {
	veryLong := strings.Repeat("verylongtoken ", 12)

	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("folding").
		SetTextBody("body").
		SetHeader("X-Long", veryLong)

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "X-Long:") {
		t.Fatalf("missing X-Long header")
	}

	if !strings.Contains(mime, "\r\n ") {
		t.Fatalf("expected folded continuation line, got:\n%s", mime)
	}
}

func TestAttachmentFilenameUsesRFC2231Parameter(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("filename").
		SetTextBody("body")

	m.AddAttachment("fakturá-číslo.pdf", []byte("content"))

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "filename*=") {
		t.Fatalf("expected filename* parameter for non-ascii name, got:\n%s", mime)
	}
}

func TestToMIMEUsesQuotedPrintableForNonASCIIBody(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("qp").
		SetTextBody("Příliš žluťoučký kůň")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("expected quoted-printable encoding, got:\n%s", mime)
	}
}

func TestToMIMEUsesSevenBitForASCIIBody(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("7bit").
		SetTextBody("Hello plain ASCII")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Content-Transfer-Encoding: 7bit") {
		t.Fatalf("expected 7bit encoding, got:\n%s", mime)
	}
}

func TestToMIMEEncodesNonASCIISubject(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("Příliš žluťoučký kůň").
		SetTextBody("body")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Subject: =?utf-8?") {
		t.Fatalf("expected RFC2047 encoded subject, got:\n%s", mime)
	}
}

func TestToMIMEEncodesAndFoldsLongNonASCIISubject(t *testing.T) {
	longSubject := strings.Repeat("Příliš žluťoučký kůň ", 8)

	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject(longSubject).
		SetTextBody("body")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Subject: =?utf-8?") {
		t.Fatalf("expected encoded subject prefix, got:\n%s", mime)
	}

	if !strings.Contains(mime, "\r\n ") {
		t.Fatalf("expected folded continuation line for long subject, got:\n%s", mime)
	}
}

func TestToMIMEDoesNotEncodeASCIISubject(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("Hello Subject").
		SetTextBody("body")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Subject: Hello Subject") {
		t.Fatalf("expected plain subject, got:\n%s", mime)
	}
}

func TestToMIMEEncodesDisplayNameInRawAddressHeader(t *testing.T) {
	m := New().
		SetFrom("Příliš Žluťoučký <sender@example.com>").
		AddTo("Žofie Recipient <recipient@example.com>").
		SetSubject("hello").
		SetTextBody("body")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "From: =?utf-8?") {
		t.Fatalf("expected encoded from display name, got:\n%s", mime)
	}

	if !strings.Contains(mime, "To: =?utf-8?") {
		t.Fatalf("expected encoded to display name, got:\n%s", mime)
	}
}

func TestToMIMEKeepsRepeatedCustomHeadersSeparated(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		AddHeader("X-Tag", "one").
		AddHeader("X-Tag", "two")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Contains(mime, "X-Tag: one, two") {
		t.Fatalf("expected separate X-Tag lines, got combined value:\n%s", mime)
	}

	if !strings.Contains(mime, "X-Tag: one") {
		t.Fatalf("missing first X-Tag value:\n%s", mime)
	}

	if !strings.Contains(mime, "X-Tag: two") {
		t.Fatalf("missing second X-Tag value:\n%s", mime)
	}
}

func TestToMIMEEncodesRepeatedNonASCIICustomHeaders(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		AddHeader("X-Note", "Příliš žluťoučký").
		AddHeader("X-Note", "kůň")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Count(mime, "X-Note: =?utf-8?") != 2 {
		t.Fatalf("expected two encoded X-Note headers, got:\n%s", mime)
	}
}

func TestToMIMESetHeaderOverridesPreviousAddHeaderValues(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		AddHeader("X-Trace", "old-1").
		AddHeader("X-Trace", "old-2").
		SetHeader("X-Trace", "final")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Count(mime, "X-Trace:") != 1 {
		t.Fatalf("expected single X-Trace header after SetHeader override, got:\n%s", mime)
	}

	if !strings.Contains(mime, "X-Trace: final") {
		t.Fatalf("expected overridden X-Trace value, got:\n%s", mime)
	}

	if strings.Contains(mime, "old-1") || strings.Contains(mime, "old-2") {
		t.Fatalf("expected previous X-Trace values to be removed, got:\n%s", mime)
	}
}

func TestToMIMECustomDateAndMessageIDOverrideGeneratedHeaders(t *testing.T) {
	customDate := "Mon, 02 Jan 2006 15:04:05 +0000"
	customMessageID := "<custom-override@example.com>"

	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		SetHeader("Date", customDate).
		SetHeader("Message-ID", customMessageID)

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Count(mime, "Date:") != 1 {
		t.Fatalf("expected single Date header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Date: "+customDate) {
		t.Fatalf("expected custom Date header, got:\n%s", mime)
	}

	if strings.Count(mime, "Message-ID:") != 1 {
		t.Fatalf("expected single Message-ID header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Message-ID: "+customMessageID) {
		t.Fatalf("expected custom Message-ID header, got:\n%s", mime)
	}
}

func TestToMIMECustomMIMEVersionOverridesGeneratedHeader(t *testing.T) {
	customMIMEVersion := "2.0"

	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		SetHeader("MIME-Version", customMIMEVersion)

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Count(mime, "Mime-Version:") != 1 {
		t.Fatalf("expected single Mime-Version header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Mime-Version: "+customMIMEVersion) {
		t.Fatalf("expected custom Mime-Version header, got:\n%s", mime)
	}
}

func TestNewMessageCopiesInputAndInitializesBody(t *testing.T) {
	from := []string{"sender@example.com"}
	to := []string{"to@example.com"}
	subject := "hello"
	body := "plain body"

	m := NewMessage(&from, &to, &subject, &body)

	from[0] = "changed@example.com"
	to[0] = "changed-to@example.com"

	if m.GetFrom()[0] != "sender@example.com" {
		t.Fatalf("expected copied from value, got: %v", m.GetFrom())
	}

	if m.GetTo()[0] != "to@example.com" {
		t.Fatalf("expected copied to value, got: %v", m.GetTo())
	}

	if m.GetSubject() != subject {
		t.Fatalf("unexpected subject: %s", m.GetSubject())
	}

	if m.GetBody() != body || m.TextBody != body {
		t.Fatalf("expected body/text body to be initialized, got body=%q text=%q", m.GetBody(), m.TextBody)
	}
}

func TestMessageCloneDeepCopy(t *testing.T) {
	original := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("clone").
		SetTextBody("body").
		AddHeader("X-Tag", "one")
	original.AddAttachment("doc.txt", []byte("payload"))

	cloned := original.Clone()
	if cloned == nil {
		t.Fatalf("expected non-nil clone")
	}

	cloned.From[0] = "mutated@example.com"
	cloned.To[0] = "mutated-to@example.com"
	cloned.Headers["X-Tag"][0] = "changed"
	cloned.Attachments[0].Data[0] = 'X'

	if original.From[0] != "sender@example.com" {
		t.Fatalf("original from should remain unchanged")
	}

	if original.To[0] != "recipient@example.com" {
		t.Fatalf("original to should remain unchanged")
	}

	if original.Headers["X-Tag"][0] != "one" {
		t.Fatalf("original header should remain unchanged")
	}

	if string(original.Attachments[0].Data) != "payload" {
		t.Fatalf("original attachment bytes should remain unchanged")
	}
}

func TestMetadataHelpersEdgeCases(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("meta").
		SetTextBody("body")

	m.SetOrganization("   ")
	m.SetReadReceiptTo("invalid")
	m.SetReturnPath("invalid")
	m.SetPriority(99)
	m.SetDate(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	m.SetMessageID(" custom@id ")
	m.SetListUnsubscribe("", "   ")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if strings.Contains(mime, "Organization:") {
		t.Fatalf("organization should not be set for blank input")
	}

	if strings.Contains(mime, "Disposition-Notification-To:") || strings.Contains(mime, "Return-Receipt-To:") {
		t.Fatalf("read receipt headers should not be set for invalid input")
	}

	if strings.Contains(mime, "Return-Path:") {
		t.Fatalf("return-path should not be set for invalid input")
	}

	if !strings.Contains(mime, "X-Priority: 5") || !strings.Contains(mime, "Importance: Low") || !strings.Contains(mime, "Priority: non-urgent") {
		t.Fatalf("expected clamped low priority headers, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Date: Tue, 02 Jan 2024 03:04:05 +0000") {
		t.Fatalf("expected explicit Date header, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Message-ID: <custom@id>") {
		t.Fatalf("expected normalized Message-ID, got:\n%s", mime)
	}

	if strings.Contains(mime, "List-Unsubscribe:") {
		t.Fatalf("list-unsubscribe should not be set for empty values")
	}
}

func TestMessageMetadataHelpersAdditionalBranches(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("meta").
		SetTextBody("body")

	m.SetHeader("X-Test", "first")
	copyValues := m.GetHeader("X-Test")
	copyValues[0] = "changed"
	if got := m.GetHeader("X-Test"); got[0] != "first" {
		t.Fatalf("GetHeader should return a copy, got %v", got)
	}

	m.SetMessageID("")
	m.SetMessageID("<already@example.com>")
	m.SetPriority(3)

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "Message-ID: <already@example.com>") {
		t.Fatalf("expected preserved message-id, got:\n%s", mime)
	}

	if !strings.Contains(mime, "X-Priority: 3") || !strings.Contains(mime, "Importance: Normal") || !strings.Contains(mime, "Priority: normal") {
		t.Fatalf("expected normal priority headers, got:\n%s", mime)
	}
}

func TestSetHeaderInitializesMapAndPriorityClampsLow(t *testing.T) {
	var m Message
	m.SetHeader("X-Test", "value")
	if got := m.GetHeader("X-Test"); len(got) != 1 || got[0] != "value" {
		t.Fatalf("expected SetHeader to initialize header map, got %v", got)
	}

	m.SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("prio").SetTextBody("body").SetPriority(0)
	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}
	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "X-Priority: 1") || !strings.Contains(mime, "Importance: High") || !strings.Contains(mime, "Priority: urgent") {
		t.Fatalf("expected low clamp to priority 1/high/urgent, got:\n%s", mime)
	}
}

func TestToMIMEIgnoresInvalidHeaderNames(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body")

	m.AddHeader("X-Good", "ok")
	m.AddHeader("Bad\r\nInjected", "boom")
	m.SetHeader("Also:Bad", "boom")
	m.Headers["Raw\nInjected"] = []string{"boom"}

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "X-Good: ok") {
		t.Fatalf("expected valid header to remain, got:\n%s", mime)
	}

	if strings.Contains(mime, "Injected") || strings.Contains(mime, "Also:Bad") {
		t.Fatalf("expected invalid header names to be skipped, got:\n%s", mime)
	}
}

func TestToMIMESanitizesHeaderValuesAndInvalidRawAddresses(t *testing.T) {
	m := New().
		SetFrom("Sender <sender@example.com>\r\nX-Evil: 1").
		AddTo("Recipient <recipient@example.com>\r\nBcc: victim@example.com").
		SetSubject("Hello\r\nBcc: victim@example.com").
		SetTextBody("body")

	m.AddHeader("X-Test", "ok\r\nInjected: yes")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Contains(mime, "\r\nBcc: victim@example.com") || strings.Contains(mime, "\r\nX-Evil: 1") || strings.Contains(mime, "\r\nInjected: yes") {
		t.Fatalf("expected header injection payloads to be neutralized, got:\n%s", mime)
	}

	if strings.Contains(mime, "From:") {
		t.Fatalf("expected invalid raw From value to be omitted, got:\n%s", mime)
	}

	if strings.Contains(mime, "To:") {
		t.Fatalf("expected invalid raw To value to be omitted, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Subject: Hello Bcc: victim@example.com") {
		t.Fatalf("expected subject to be normalized to one line, got:\n%s", mime)
	}

	if !strings.Contains(mime, "X-Test: ok Injected: yes") {
		t.Fatalf("expected custom header value to be normalized to one line, got:\n%s", mime)
	}
}

func TestToMIMESkipsBlankCustomHeaderValuesAndUsesLocalhostMessageIDFallback(t *testing.T) {
	m := New().
		SetFrom("not a mailbox").
		AddTo("recipient@example.com").
		SetSubject("headers").
		SetTextBody("body").
		AddHeader("X-Blank", "   ")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if strings.Contains(mime, "X-Blank:") {
		t.Fatalf("expected blank custom header to be skipped, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Message-ID: <") || !strings.Contains(mime, "@localhost>") {
		t.Fatalf("expected localhost message-id fallback, got:\n%s", mime)
	}
}

func TestAddAttachmentFromPathAndEmbeddedFile(t *testing.T) {
	tmpDir := t.TempDir()
	attachmentPath := filepath.Join(tmpDir, "report.txt")
	embeddedPath := filepath.Join(tmpDir, "logo.png")

	if err := os.WriteFile(attachmentPath, []byte("report-data"), 0o600); err != nil {
		t.Fatalf("write attachment failed: %v", err)
	}

	if err := os.WriteFile(embeddedPath, []byte("png-data"), 0o600); err != nil {
		t.Fatalf("write embedded file failed: %v", err)
	}

	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("files").
		SetTextBody("body")

	if _, err := m.AddAttachmentFromPath(attachmentPath); err != nil {
		t.Fatalf("AddAttachmentFromPath failed: %v", err)
	}

	if _, err := m.AddEmbeddedFile(embeddedPath, "logo-cid"); err != nil {
		t.Fatalf("AddEmbeddedFile failed: %v", err)
	}

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "filename=report.txt") {
		t.Fatalf("expected attachment from path filename, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Content-Id: <logo-cid>") {
		t.Fatalf("expected embedded content id, got:\n%s", mime)
	}
}

func TestEnvelopeFromFallbackAndMissingSender(t *testing.T) {
	m := New().SetFrom("Sender <sender@example.com>")

	envelope, err := m.envelopeFrom()
	if err != nil {
		t.Fatalf("envelopeFrom failed: %v", err)
	}

	if envelope != "sender@example.com" {
		t.Fatalf("unexpected envelope from: %s", envelope)
	}

	empty := New()
	if _, err := empty.envelopeFrom(); err == nil {
		t.Fatalf("expected envelopeFrom to fail without sender")
	}
}

func TestToMIMEUsesHTMLBodyWhenOnlyHTMLProvided(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("html").
		SetHtmlBody("<h1>Hello</h1>")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := string(raw)
	if !strings.Contains(mime, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected text/html body content type, got:\n%s", mime)
	}
}

func TestToMIMEUsesMultipartAlternativeForTextAndHTML(t *testing.T) {
	m := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("alt").
		SetTextBody("plain").
		SetHtmlBody("<p>html</p>")

	raw, err := m.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	mime := unfoldHeaderContinuations(string(raw))
	if !strings.Contains(mime, "Content-Type: multipart/alternative;") {
		t.Fatalf("expected multipart/alternative body, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("expected text/plain part, got:\n%s", mime)
	}

	if !strings.Contains(mime, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected text/html part, got:\n%s", mime)
	}
}

func TestMailboxSetterGetterCoverage(t *testing.T) {
	m := New()
	m.SetFrom("sender@example.com").AddFrom("extra@example.com")
	m.SetTo([]string{"to1@example.com"})
	m.SetCc([]string{"cc1@example.com"}).AddCc("cc2@example.com")
	m.SetBcc([]string{"bcc1@example.com"}).AddBcc("bcc2@example.com")
	m.SetReplyTo([]string{"reply1@example.com"}).AddReplyTo("reply2@example.com")
	m.SetSubject("subject")
	m.SetBody("body")

	if len(m.GetBcc()) != 2 {
		t.Fatalf("expected 2 bcc values, got %v", m.GetBcc())
	}

	if len(m.GetReplyTo()) != 2 {
		t.Fatalf("expected 2 reply-to values, got %v", m.GetReplyTo())
	}

	if m.GetSubject() != "subject" || m.GetBody() != "body" {
		t.Fatalf("unexpected subject/body getters")
	}
}

func TestMailboxAddressCollectionMethodsCoverage(t *testing.T) {
	m := New()
	bcc, _ := NewAddress("bcc@example.com", "Bcc")
	reply, _ := NewAddress("reply@example.com", "Reply")

	if _, err := m.SetBccMailboxes([]Address{bcc}); err != nil {
		t.Fatalf("SetBccMailboxes failed: %v", err)
	}
	if _, err := m.AddBccMailboxes(bcc); err != nil {
		t.Fatalf("AddBccMailboxes failed: %v", err)
	}
	if _, err := m.SetReplyToMailboxes([]Address{reply}); err != nil {
		t.Fatalf("SetReplyToMailboxes failed: %v", err)
	}
	if _, err := m.AddReplyToMailboxes(reply); err != nil {
		t.Fatalf("AddReplyToMailboxes failed: %v", err)
	}

	if _, err := m.AddFromAddress("from@example.com", "From"); err != nil {
		t.Fatalf("AddFromAddress failed: %v", err)
	}
	if _, err := m.AddFromMailbox(Address{Email: "broken", Name: "Bad"}); err == nil {
		t.Fatalf("expected AddFromMailbox to fail for invalid email")
	}

	if _, err := m.AddBccAddress("broken", "Bad"); err == nil {
		t.Fatalf("expected AddBccAddress to fail")
	}
	if _, err := m.AddReplyToAddress("broken", "Bad"); err == nil {
		t.Fatalf("expected AddReplyToAddress to fail")
	}
}

func TestUtilityFunctionCoverage(t *testing.T) {
	if got := formatAddressHeaderValues([]string{" ", "Name <a@example.com>", "invalid"}); !strings.Contains(got, "a@example.com") || strings.Contains(got, "invalid") {
		t.Fatalf("unexpected formatted address header values: %q", got)
	}

	if got := formatContentDisposition("attachment", ""); got != "attachment" {
		t.Fatalf("expected plain disposition for empty filename, got %q", got)
	}
	if got := formatContentDisposition("attachment", "ascii.txt"); !strings.Contains(got, "filename=ascii.txt") {
		t.Fatalf("expected ascii filename disposition, got %q", got)
	}
	if got := formatContentDisposition("attachment", "žluťoučký.txt"); !strings.Contains(got, "filename*=utf-8''") {
		t.Fatalf("expected rfc2231 filename* disposition, got %q", got)
	}

	if !strings.Contains(mediaType("text/plain", map[string]string{"charset": "UTF-8"}), "charset=UTF-8") {
		t.Fatalf("expected media type params")
	}
	if got := mediaType("", map[string]string{"x": "y"}); got != "" {
		t.Fatalf("expected empty base type fallback, got %q", got)
	}

	if got := encodeHeaderValue("X-Test", "Příliš"); !strings.Contains(got, "=?utf-8?") {
		t.Fatalf("expected encoded header value, got %q", got)
	}
	if got := encodeHeaderValue("Date", "Příliš"); got != "Příliš" {
		t.Fatalf("expected Date to remain unencoded, got %q", got)
	}

	if encoding, body, err := encodeTextBody("ASCII line"); err != nil || encoding != "7bit" || string(body) != "ASCII line" {
		t.Fatalf("expected 7bit body, got encoding=%q err=%v body=%q", encoding, err, string(body))
	}
	if encoding, _, err := encodeTextBody("Příliš"); err != nil || encoding != "quoted-printable" {
		t.Fatalf("expected quoted-printable body, got encoding=%q err=%v", encoding, err)
	}

	if !isBodySevenBitSafe(strings.Repeat("a", 998)) {
		t.Fatalf("expected 998-char line to be valid")
	}
	if isBodySevenBitSafe(strings.Repeat("a", 999)) {
		t.Fatalf("expected 999-char line to be invalid")
	}
	if isBodySevenBitSafe("Příliš") {
		t.Fatalf("expected non-ascii body to be invalid for 7bit")
	}

	if string(cloneBytes(nil)) != "" {
		t.Fatalf("expected nil clone bytes to stay nil")
	}

	if _, err := formatMailbox("invalid", "Bad"); err == nil {
		t.Fatalf("expected formatMailbox to fail for invalid address")
	}
	if got, err := formatMailbox("ok@example.com", "Ok"); err != nil || got == "" {
		t.Fatalf("expected formatMailbox success, got %q err=%v", got, err)
	}
}

func TestMailboxInvalidCollectionBranchesAndHeaderMapNil(t *testing.T) {
	m := New()

	bad := Address{Email: "invalid", Name: "Bad"}
	if _, err := m.SetToMailboxes([]Address{bad}); err == nil {
		t.Fatalf("expected SetToMailboxes error")
	}
	if _, err := m.SetCcMailboxes([]Address{bad}); err == nil {
		t.Fatalf("expected SetCcMailboxes error")
	}
	if _, err := m.SetBccMailboxes([]Address{bad}); err == nil {
		t.Fatalf("expected SetBccMailboxes error")
	}
	if _, err := m.SetReplyToMailboxes([]Address{bad}); err == nil {
		t.Fatalf("expected SetReplyToMailboxes error")
	}
	if _, err := m.AddCcMailboxes(bad); err == nil {
		t.Fatalf("expected AddCcMailboxes error")
	}
	if _, err := m.AddBccMailboxes(bad); err == nil {
		t.Fatalf("expected AddBccMailboxes error")
	}
	if _, err := m.AddReplyToMailboxes(bad); err == nil {
		t.Fatalf("expected AddReplyToMailboxes error")
	}
	if _, err := m.AddToAddress("invalid", "Bad"); err == nil {
		t.Fatalf("expected AddToAddress error")
	}
	if _, err := m.AddCcAddress("invalid", "Bad"); err == nil {
		t.Fatalf("expected AddCcAddress error")
	}
	if _, err := m.AddToMailbox(bad); err == nil {
		t.Fatalf("expected AddToMailbox error")
	}
	if _, err := m.AddCcMailbox(bad); err == nil {
		t.Fatalf("expected AddCcMailbox error")
	}
	if _, err := m.AddBccMailbox(bad); err == nil {
		t.Fatalf("expected AddBccMailbox error")
	}
	if _, err := m.AddReplyToMailbox(bad); err == nil {
		t.Fatalf("expected AddReplyToMailbox error")
	}

	m.Headers = nil
	m.AddHeader("X-Map", "v1")
	m.SetHeader("X-Map", "v2")
	if values := m.GetHeader("X-Map"); len(values) != 1 || values[0] != "v2" {
		t.Fatalf("expected SetHeader to reset value, got %v", values)
	}

	m.Headers = nil
	if got := m.GetHeader("X-Unknown"); got != nil {
		t.Fatalf("expected nil header from nil header map")
	}

	m.Headers = map[string][]string{"X-One": {"1"}}
	if got := m.GetHeader("X-Missing"); got != nil {
		t.Fatalf("expected nil for missing header, got %v", got)
	}
}

func TestMessageIDAndFileErrorBranches(t *testing.T) {
	m := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetTextBody("b")
	m.SetMessageID(" id-only-prefix")
	if got := m.GetHeader("Message-ID"); len(got) != 1 || !strings.HasPrefix(got[0], "<") {
		t.Fatalf("expected normalized message-id with prefix, got %v", got)
	}
	m.SetMessageID("id-only-suffix ")
	if got := m.GetHeader("Message-ID"); len(got) != 1 || !strings.HasSuffix(got[0], ">") {
		t.Fatalf("expected normalized message-id with suffix, got %v", got)
	}

	if _, err := m.AddAttachmentFromPath(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatalf("expected AddAttachmentFromPath error for missing file")
	}
	if _, err := m.AddEmbeddedFile(filepath.Join(t.TempDir(), "missing.png"), "cid"); err == nil {
		t.Fatalf("expected AddEmbeddedFile error for missing file")
	}
}

func unfoldHeaderContinuations(input string) string {
	replacer := strings.NewReplacer("\r\n ", " ", "\r\n\t", " ")
	return replacer.Replace(input)
}
