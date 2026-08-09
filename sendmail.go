package mailer

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type SendmailMailer struct {
	path         string
	args         []string
	interceptors []Interceptor
	dkimSigner   *DKIMSigner
}

func NewSendmailMailer(path string, args ...string) *SendmailMailer {
	mailerPath := strings.TrimSpace(path)
	if mailerPath == "" {
		mailerPath = "/usr/sbin/sendmail"
	}

	mailerArgs := append([]string(nil), args...)
	if len(mailerArgs) == 0 {
		mailerArgs = []string{"-i", "-t"}
	}

	return &SendmailMailer{
		path: mailerPath,
		args: mailerArgs,
	}
}

func (m *SendmailMailer) Use(interceptor Interceptor) *SendmailMailer {
	if interceptor != nil {
		m.interceptors = append(m.interceptors, interceptor)
	}
	return m
}

func (m *SendmailMailer) SetDKIMSigner(signer *DKIMSigner) *SendmailMailer {
	m.dkimSigner = signer
	return m
}

func (m *SendmailMailer) Send(message *Message) error {
	if message == nil {
		return fmt.Errorf("message is nil")
	}

	working := message.Clone()
	if err := applyInterceptors(working, m.interceptors); err != nil {
		return err
	}

	raw, err := working.ToMIME()
	if err != nil {
		return fmt.Errorf("build mime message: %w", err)
	}

	if m.dkimSigner != nil {
		raw, err = m.dkimSigner.Sign(raw)
		if err != nil {
			return fmt.Errorf("dkim signing failed: %w", err)
		}
	}

	args := append([]string(nil), m.args...)
	if envelopeFrom, err := working.envelopeFrom(); err == nil && envelopeFrom != "" && !hasSendmailEnvelopeArg(args) {
		args = append(args, "-f", envelopeFrom)
	}

	cmd := exec.Command(m.path, args...)
	cmd.Stdin = bytes.NewReader(raw)

	if output, err := cmd.CombinedOutput(); err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("sendmail failed: %w", err)
		}
		return fmt.Errorf("sendmail failed: %w: %s", err, trimmed)
	}

	return nil
}

func hasSendmailEnvelopeArg(args []string) bool {
	for i, arg := range args {
		if arg == "-f" && i+1 < len(args) {
			return true
		}
		if strings.HasPrefix(arg, "-f") && len(arg) > 2 {
			return true
		}
	}

	return false
}
