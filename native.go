package mailer

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type NativeMailer struct {
	sendmail *SendmailMailer
}

func NewNativeMailer() (*NativeMailer, error) {
	path, err := resolveNativeSendmailPath(runtime.GOOS)
	if err != nil {
		return nil, err
	}

	return NewNativeMailerWithPath(path), nil
}

func NewNativeMailerWithPath(path string) *NativeMailer {
	return &NativeMailer{
		sendmail: NewSendmailMailer(path),
	}
}

func (m *NativeMailer) Use(interceptor Interceptor) *NativeMailer {
	m.sendmail.Use(interceptor)
	return m
}

func (m *NativeMailer) SetDKIMSigner(signer *DKIMSigner) *NativeMailer {
	m.sendmail.SetDKIMSigner(signer)
	return m
}

func (m *NativeMailer) Send(message *Message) error {
	if m == nil || m.sendmail == nil {
		return fmt.Errorf("native mailer is not configured")
	}

	return m.sendmail.Send(message)
}

func resolveNativeSendmailPath(goos string) (string, error) {
	for _, candidate := range nativeSendmailCandidates(goos) {
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}

	if discovered, err := exec.LookPath("sendmail"); err == nil {
		return discovered, nil
	}

	return "", fmt.Errorf("native sendmail transport is not available on %s", goos)
}

func nativeSendmailCandidates(goos string) []string {
	switch goos {
	case "linux":
		return []string{"/usr/sbin/sendmail", "/usr/lib/sendmail", "/usr/bin/sendmail"}
	case "darwin":
		return []string{"/usr/sbin/sendmail", "/usr/bin/sendmail"}
	default:
		return nil
	}
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	return info.Mode()&0o111 != 0
}
