package mailer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeSendmailCandidates(t *testing.T) {
	linux := nativeSendmailCandidates("linux")
	if len(linux) == 0 || linux[0] != "/usr/sbin/sendmail" {
		t.Fatalf("unexpected linux candidates: %v", linux)
	}

	darwin := nativeSendmailCandidates("darwin")
	if len(darwin) == 0 || darwin[0] != "/usr/sbin/sendmail" {
		t.Fatalf("unexpected darwin candidates: %v", darwin)
	}

	windows := nativeSendmailCandidates("windows")
	if len(windows) != 0 {
		t.Fatalf("expected no candidates on windows, got: %v", windows)
	}
}

func TestNativeMailerWithPathDelegates(t *testing.T) {
	tmpDir := t.TempDir()
	messagePath := filepath.Join(tmpDir, "native-message.txt")

	scriptPath := filepath.Join(tmpDir, "fake-sendmail.sh")
	script := "#!/bin/sh\ncat > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	native := NewNativeMailerWithPath(scriptPath)
	native.sendmail.args = []string{messagePath}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("native").
		SetTextBody("body")

	if err := native.Send(message); err != nil {
		t.Fatalf("native send failed: %v", err)
	}

	raw, err := os.ReadFile(messagePath)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	if !strings.Contains(string(raw), "Subject: native") {
		t.Fatalf("expected native mail output to contain subject")
	}
}

func TestResolveNativeSendmailPathViaLookPath(t *testing.T) {
	tmpDir := t.TempDir()
	sendmailPath := filepath.Join(tmpDir, "sendmail")
	if err := os.WriteFile(sendmailPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake sendmail failed: %v", err)
	}

	t.Setenv("PATH", tmpDir)

	resolved, err := resolveNativeSendmailPath("plan9")
	if err != nil {
		t.Fatalf("resolveNativeSendmailPath failed: %v", err)
	}

	if resolved != sendmailPath {
		t.Fatalf("expected resolved path %q, got %q", sendmailPath, resolved)
	}
}

func TestIsExecutableFileAndNativeSendNilConfig(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "exec.sh")
	nonExecPath := filepath.Join(tmpDir, "plain.txt")

	if err := os.WriteFile(execPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable failed: %v", err)
	}

	if err := os.WriteFile(nonExecPath, []byte("text"), 0o644); err != nil {
		t.Fatalf("write non-executable failed: %v", err)
	}

	if !isExecutableFile(execPath) {
		t.Fatalf("expected executable file to be detected")
	}

	if isExecutableFile(nonExecPath) {
		t.Fatalf("expected non-executable file to be rejected")
	}

	if isExecutableFile(tmpDir) {
		t.Fatalf("expected directory to be rejected")
	}

	var native *NativeMailer
	if err := native.Send(New()); err == nil {
		t.Fatalf("expected error for nil-configured native mailer")
	}
}

func TestNativeMailerUseAndSetDKIMSigner(t *testing.T) {
	native := NewNativeMailerWithPath("/bin/true")
	native.Use(nil)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	if native.SetDKIMSigner(signer) != native {
		t.Fatalf("expected SetDKIMSigner to be chainable")
	}
}

func TestNewNativeMailerEntryPoint(t *testing.T) {
	native, err := NewNativeMailer()
	if err != nil {
		if native != nil {
			t.Fatalf("expected nil native mailer on error")
		}
		return
	}

	if native == nil {
		t.Fatalf("expected native mailer when no error")
	}
}

func TestResolveNativeSendmailPathErrorBranch(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := resolveNativeSendmailPath("plan9"); err == nil {
		t.Fatalf("expected native resolver error when no candidates and PATH is empty")
	}
}
