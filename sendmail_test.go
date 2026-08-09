package mailer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendmailMailerSend(t *testing.T) {
	tmpDir := t.TempDir()
	messagePath := filepath.Join(tmpDir, "message.txt")

	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		SetReturnPath("bounce@example.com").
		AddTo("recipient@example.com").
		SetSubject("sendmail test").
		SetTextBody("hello")

	mailer := NewSendmailMailer(scriptPath, messagePath).Use(func(m *Message) error {
		m.SetHeader("X-Intercepted", "yes")
		return nil
	})

	if err := mailer.Send(message); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	raw, err := os.ReadFile(messagePath)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	text := string(raw)
	if !strings.Contains(text, "Subject: sendmail test") {
		t.Fatalf("message does not contain subject")
	}

	if !strings.Contains(text, "X-Intercepted: yes") {
		t.Fatalf("message does not contain interceptor header")
	}
}

func TestSendmailMailerAddsEnvelopeFromArg(t *testing.T) {
	tmpDir := t.TempDir()
	messagePath := filepath.Join(tmpDir, "message.txt")
	argsPath := filepath.Join(tmpDir, "args.txt")

	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$2\"\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		SetReturnPath("bounce@example.com").
		AddTo("recipient@example.com").
		SetSubject("sendmail args").
		SetTextBody("hello")

	mailer := NewSendmailMailer(scriptPath, messagePath, argsPath)
	if err := mailer.Send(message); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args failed: %v", err)
	}

	argsText := string(argsRaw)
	if !strings.Contains(argsText, "-f") || !strings.Contains(argsText, "bounce@example.com") {
		t.Fatalf("expected -f envelope sender in args, got: %s", argsText)
	}
}

func TestSendmailMailerDKIM(t *testing.T) {
	tmpDir := t.TempDir()
	messagePath := filepath.Join(tmpDir, "message.txt")

	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("sendmail dkim").
		SetTextBody("hello")

	mailer := NewSendmailMailer(scriptPath, messagePath).SetDKIMSigner(signer)
	if err := mailer.Send(message); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	raw, err := os.ReadFile(messagePath)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	if !strings.HasPrefix(string(raw), "DKIM-Signature: ") {
		t.Fatalf("expected dkim header")
	}
}

func TestNewSendmailMailerDefaultsAndEnvelopeArgHelper(t *testing.T) {
	m := NewSendmailMailer("   ")
	if m.path != "/usr/sbin/sendmail" {
		t.Fatalf("expected default sendmail path, got %q", m.path)
	}
	if len(m.args) != 2 || m.args[0] != "-i" || m.args[1] != "-t" {
		t.Fatalf("unexpected default args: %v", m.args)
	}

	if !hasSendmailEnvelopeArg([]string{"-f", "bounce@example.com"}) {
		t.Fatalf("expected -f separated syntax to be recognized")
	}
	if !hasSendmailEnvelopeArg([]string{"-fbounce@example.com"}) {
		t.Fatalf("expected -f inline syntax to be recognized")
	}
	if hasSendmailEnvelopeArg([]string{"-f"}) {
		t.Fatalf("expected lone -f to be ignored")
	}
}

func TestSendmailMailerErrors(t *testing.T) {
	mailer := NewSendmailMailer("/bin/true")
	if err := mailer.Send(nil); err == nil {
		t.Fatalf("expected nil message error")
	}

	tmpDir := t.TempDir()
	failingScript := filepath.Join(tmpDir, "fail-sendmail.sh")
	script := "#!/bin/sh\necho 'boom' >&2\nexit 12\n"
	if err := os.WriteFile(failingScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")
	if err := NewSendmailMailer(failingScript).Send(msg); err == nil || !strings.Contains(err.Error(), "sendmail failed") {
		t.Fatalf("expected sendmail command failure, got %v", err)
	}

	silentFailingScript := filepath.Join(tmpDir, "silent-fail-sendmail.sh")
	silentScript := "#!/bin/sh\nexit 12\n"
	if err := os.WriteFile(silentFailingScript, []byte(silentScript), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	if err := NewSendmailMailer(silentFailingScript).Send(msg); err == nil || strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected silent sendmail failure without stderr output, got %v", err)
	}
}

func TestSendmailMailerPreservesExistingEnvelopeArgAndDKIMError(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "args.txt")
	messagePath := filepath.Join(tmpDir, "message.txt")
	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$2\"\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	msg := New().SetFrom("sender@example.com").SetReturnPath("bounce@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")
	mailer := NewSendmailMailer(scriptPath, messagePath, argsPath, "-fbounce@example.com")
	if err := mailer.Send(msg); err != nil {
		t.Fatalf("send with existing envelope arg failed: %v", err)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args failed: %v", err)
	}
	if strings.Count(string(argsRaw), "bounce@example.com") != 1 {
		t.Fatalf("expected existing envelope arg to be preserved without duplication, got: %s", string(argsRaw))
	}

	badDKIMMailer := NewSendmailMailer(scriptPath, messagePath).SetDKIMSigner(&DKIMSigner{})
	if err := badDKIMMailer.Send(msg); err == nil || !strings.Contains(err.Error(), "dkim signing failed") {
		t.Fatalf("expected dkim signing failure, got %v", err)
	}
}

func TestSendmailMailerInterceptorFailure(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\ncat >/dev/null\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")
	mailer := NewSendmailMailer(scriptPath).Use(func(m *Message) error {
		return errors.New("interceptor failed")
	})
	if err := mailer.Send(msg); err == nil || !strings.Contains(err.Error(), "interceptor failed") {
		t.Fatalf("expected interceptor failure, got %v", err)
	}
}

func TestSendmailMailerWithoutResolvableEnvelopeFrom(t *testing.T) {
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "args.txt")
	messagePath := filepath.Join(tmpDir, "message.txt")
	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"$2\"\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	msg := New().
		AddHeader("X-Test", "value").
		SetSubject("s").
		SetTextBody("b")

	mailer := NewSendmailMailer(scriptPath, messagePath, argsPath)
	if err := mailer.Send(msg); err != nil {
		t.Fatalf("send without resolvable envelope from failed: %v", err)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args failed: %v", err)
	}
	if strings.Contains(string(argsRaw), "-f") {
		t.Fatalf("did not expect auto envelope arg without valid sender, got: %s", string(argsRaw))
	}
}
