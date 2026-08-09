package mailer

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type smtpTestServer struct {
	address  string
	listener net.Listener

	failMail bool
	greeting string
	ehloCode string
	heloCode string
	rcptCode string
	dataCode string
	quitCode string

	mu          sync.Mutex
	commands    []string
	messageData string
}

func startSMTPTestServer(t *testing.T, failMail bool) *smtpTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test smtp listener: %v", err)
	}

	server := &smtpTestServer{
		address:  ln.Addr().String(),
		listener: ln,
		failMail: failMail,
		greeting: "220 localhost ESMTP ready",
		ehloCode: "250-localhost\r\n250 SIZE 10485760",
		heloCode: "250 localhost",
		rcptCode: "250 ok",
		dataCode: "250 queued",
		quitCode: "221 bye",
	}

	go server.serve(t)

	return server
}

func startSMTPScenarioServer(t *testing.T, configure func(*smtpTestServer)) *smtpTestServer {
	t.Helper()
	server := startSMTPTestServer(t, false)
	if configure != nil {
		configure(server)
	}
	return server
}

func (s *smtpTestServer) close() {
	_ = s.listener.Close()
}

func (s *smtpTestServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portRaw, err := net.SplitHostPort(s.address)
	if err != nil {
		t.Fatalf("split host/port failed: %v", err)
	}

	var port int
	if _, err = fmt.Sscanf(portRaw, "%d", &port); err != nil {
		t.Fatalf("parse port failed: %v", err)
	}

	return host, port
}

func (s *smtpTestServer) serve(t *testing.T) {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	_, _ = w.WriteString(s.greeting + "\r\n")
	_ = w.Flush()

	for {
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		s.recordCommand(line)

		switch {
		case strings.HasPrefix(line, "EHLO "):
			_, _ = w.WriteString(s.ehloCode + "\r\n")
			_ = w.Flush()
		case strings.HasPrefix(line, "HELO "):
			_, _ = w.WriteString(s.heloCode + "\r\n")
			_ = w.Flush()
		case strings.HasPrefix(line, "MAIL FROM:"):
			if s.failMail {
				_, _ = w.WriteString("550 sender rejected\r\n")
			} else {
				_, _ = w.WriteString("250 ok\r\n")
			}
			_ = w.Flush()
		case strings.HasPrefix(line, "RCPT TO:"):
			_, _ = w.WriteString(s.rcptCode + "\r\n")
			_ = w.Flush()
		case line == "DATA":
			_, _ = w.WriteString("354 end with <CR><LF>.<CR><LF>\r\n")
			_ = w.Flush()

			dataLines := make([]string, 0, 32)
			for {
				dataLine, dataErr := r.ReadString('\n')
				if dataErr != nil {
					return
				}
				dataLine = strings.TrimRight(dataLine, "\r\n")
				if dataLine == "." {
					break
				}
				if strings.HasPrefix(dataLine, "..") {
					dataLine = dataLine[1:]
				}
				dataLines = append(dataLines, dataLine)
			}
			s.recordMessage(strings.Join(dataLines, "\n"))

			_, _ = w.WriteString(s.dataCode + "\r\n")
			_ = w.Flush()
		case line == "QUIT":
			_, _ = w.WriteString(s.quitCode + "\r\n")
			_ = w.Flush()
			return
		default:
			_, _ = w.WriteString("250 ok\r\n")
			_ = w.Flush()
		}
	}
}

func (s *smtpTestServer) recordCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, command)
}

func (s *smtpTestServer) recordMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageData = message
}

func (s *smtpTestServer) hasCommand(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (s *smtpTestServer) message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messageData
}

func TestSMTPMailerSend(t *testing.T) {
	server := startSMTPTestServer(t, false)
	defer server.close()

	host, port := server.hostPort(t)

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("SMTP test").
		SetTextBody("hello smtp")

	mailer := NewSMTPMailer(host, port).
		SetSecurity(SecurityNone).
		SetTimeout(2 * time.Second)

	if err := mailer.Send(message); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !server.hasCommand("MAIL FROM:") {
		t.Fatalf("expected MAIL FROM command")
	}

	if !strings.Contains(server.message(), "Subject: SMTP test") {
		t.Fatalf("expected message data to include subject, got:\n%s", server.message())
	}
}

func TestSMTPMailerFailover(t *testing.T) {
	failing := startSMTPTestServer(t, true)
	defer failing.close()

	ok := startSMTPTestServer(t, false)
	defer ok.close()

	failingHost, failingPort := failing.hostPort(t)
	okHost, okPort := ok.hostPort(t)

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("Failover test").
		SetTextBody("hello")

	mailer := NewSMTPMailer(failingHost, failingPort).
		AddServer(okHost, okPort).
		SetSecurity(SecurityNone).
		SetTimeout(2 * time.Second)

	if err := mailer.Send(message); err != nil {
		t.Fatalf("expected failover send to succeed, got: %v", err)
	}

	if !failing.hasCommand("MAIL FROM:") {
		t.Fatalf("expected first server to be tried")
	}

	if !ok.hasCommand("MAIL FROM:") {
		t.Fatalf("expected second server to be tried")
	}

	if !strings.Contains(ok.message(), "Subject: Failover test") {
		t.Fatalf("expected second server to receive message")
	}
}

func TestSMTPMailerAllServersFailedAggregation(t *testing.T) {
	first := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.ehloCode = "550 no ehlo"
		s.heloCode = "550 no helo"
	})
	defer first.close()

	second := startSMTPTestServer(t, true)
	defer second.close()

	firstHost, firstPort := first.hostPort(t)
	secondHost, secondPort := second.hostPort(t)

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("aggregate").
		SetTextBody("body")

	mailer := NewSMTPMailer(firstHost, firstPort).
		AddServer(secondHost, secondPort).
		SetSecurity(SecurityNone).
		SetTimeout(2 * time.Second)

	err := mailer.Send(message)
	if err == nil {
		t.Fatalf("expected aggregated failure")
	}

	if !strings.Contains(err.Error(), "all smtp servers failed") || !strings.Contains(err.Error(), firstHost) || !strings.Contains(err.Error(), secondHost) {
		t.Fatalf("expected aggregated server failures, got %v", err)
	}
}

func TestSMTPMailerSettersAndServerSelection(t *testing.T) {
	m := NewSMTPMailer(" primary.example.com ", 25)

	customServers := []SMTPServer{{Host: " smtp1.example.com ", Port: 25}, {Host: "smtp1.example.com", Port: 25}, {Host: "", Port: 25}, {Host: "smtp2.example.com", Port: 587}}
	m.SetServers(customServers).
		SetAuth(" user@example.com ", "secret").
		SetInsecureSkipVerify(true).
		SetTimeout(-1).
		SetTimeout(3 * time.Second).
		SetSecurity(SecurityStartTLS)

	if m.host != "smtp1.example.com" || m.port != 25 {
		t.Fatalf("unexpected primary server: host=%q port=%d", m.host, m.port)
	}

	if m.username != "user@example.com" || m.password != "secret" {
		t.Fatalf("unexpected auth config: username=%q password=%q", m.username, m.password)
	}

	if !m.insecureSkipVerify {
		t.Fatalf("expected insecure skip verify to be enabled")
	}

	if m.timeout != 3*time.Second {
		t.Fatalf("expected timeout to be updated to 3s, got %v", m.timeout)
	}

	servers := m.getServers()
	if len(servers) != 2 {
		t.Fatalf("expected deduplicated valid servers, got: %v", servers)
	}

	if servers[0].Host != "smtp1.example.com" || servers[1].Host != "smtp2.example.com" {
		t.Fatalf("unexpected deduplicated servers: %v", servers)
	}
}

func TestNormalizeRecipientsAndSMTPValidationErrors(t *testing.T) {
	recipients, err := normalizeRecipients([]string{"A <a@example.com>", "a@example.com", "b@example.com"})
	if err != nil {
		t.Fatalf("normalizeRecipients failed: %v", err)
	}

	if len(recipients) != 2 {
		t.Fatalf("expected duplicate recipients to be removed, got: %v", recipients)
	}

	if _, err := normalizeRecipients([]string{"invalid"}); err == nil {
		t.Fatalf("expected invalid recipient error")
	}

	m := NewSMTPMailer("", 0).SetSecurity(SecurityNone)
	if err := m.Send(nil); err == nil || !strings.Contains(err.Error(), "message is nil") {
		t.Fatalf("expected nil message validation error, got %v", err)
	}

	message := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")
	if err := m.Send(message); err == nil || !strings.Contains(err.Error(), "smtp host is required") {
		t.Fatalf("expected host validation error, got %v", err)
	}
}

func TestSMTPEffectiveSecurityAndTLSConfig(t *testing.T) {
	m := NewSMTPMailer("smtp.example.com", 25)
	if m.effectiveSecurity(25) != SecurityAuto {
		t.Fatalf("expected auto security on port 25")
	}
	if m.effectiveSecurity(465) != SecurityTLS {
		t.Fatalf("expected implicit TLS on port 465")
	}
	m.SetSecurity(SecurityNone)
	if m.effectiveSecurity(465) != SecurityNone {
		t.Fatalf("explicit security mode should win")
	}

	m.SetInsecureSkipVerify(true)
	cfg := m.tlsConfig("smtp.example.com")
	if cfg.ServerName != "smtp.example.com" || !cfg.InsecureSkipVerify {
		t.Fatalf("unexpected tls config: %+v", cfg)
	}
}

func TestSMTPClientHelpersAndReadResponseErrors(t *testing.T) {
	client := &smtpClient{capabilities: []string{"SIZE 100", "AUTH PLAIN"}}
	if !client.hasCapability("AUTH") {
		t.Fatalf("expected AUTH capability")
	}
	if client.hasCapability("STARTTLS") {
		t.Fatalf("did not expect STARTTLS capability")
	}

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if (&smtpClient{conn: c1}).isTLS() {
		t.Fatalf("expected plain pipe conn to not be tls")
	}

	errClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("xx\r\n"))}
	if _, _, err := errClient.readResponse(); err == nil {
		t.Fatalf("expected invalid smtp response error")
	}

	errClient = &smtpClient{reader: bufio.NewReader(strings.NewReader("ABC test\r\n"))}
	if _, _, err := errClient.readResponse(); err == nil {
		t.Fatalf("expected invalid smtp status error")
	}

	okClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("250-test\r\n250 ok\r\n"))}
	code, lines, err := okClient.readResponse()
	if err != nil || code != 250 || len(lines) != 2 {
		t.Fatalf("expected multiline response parse, got code=%d lines=%v err=%v", code, lines, err)
	}
}

func TestSMTPClientWriteMessageDataAndCommands(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	client := &smtpClient{writer: bufio.NewWriter(buf)}
	if err := client.writeMessageData([]byte("line1\n.dot\n")); err != nil {
		t.Fatalf("writeMessageData failed: %v", err)
	}

	written := buf.String()
	if !strings.Contains(written, "..dot\r\n") || !strings.HasSuffix(written, ".\r\n") {
		t.Fatalf("unexpected dot-stuffed payload: %q", written)
	}

	connA, connB := net.Pipe()
	defer connA.Close()
	defer connB.Close()

	go func() {
		_, _ = io.ReadAll(connB)
	}()

	commandClient := &smtpClient{conn: connA, writer: bufio.NewWriter(connA), reader: bufio.NewReader(strings.NewReader("250 ok\r\n"))}
	if code, _, err := commandClient.command("NOOP"); err != nil || code != 250 {
		t.Fatalf("expected command success, got code=%d err=%v", code, err)
	}

	mailClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("250 ok\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := mailClient.mail("sender@example.com"); err != nil {
		t.Fatalf("mail should accept 250, got %v", err)
	}
	mailClient = &smtpClient{reader: bufio.NewReader(strings.NewReader("550 bad\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := mailClient.mail("sender@example.com"); err == nil {
		t.Fatalf("mail should reject non-250")
	}

	rcptClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("251 forward\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := rcptClient.rcpt("to@example.com"); err != nil {
		t.Fatalf("rcpt should accept 251, got %v", err)
	}
	rcptClient = &smtpClient{reader: bufio.NewReader(strings.NewReader("550 bad\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := rcptClient.rcpt("to@example.com"); err == nil {
		t.Fatalf("rcpt should reject 550")
	}

	quitClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("221 bye\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := quitClient.quit(); err != nil {
		t.Fatalf("quit should accept 221, got %v", err)
	}
	quitClient = &smtpClient{reader: bufio.NewReader(strings.NewReader("250 ok\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := quitClient.quit(); err == nil {
		t.Fatalf("quit should reject non-221")
	}

	dataClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("354 go\r\n250 ok\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := dataClient.data([]byte("hello")); err != nil {
		t.Fatalf("data should succeed, got %v", err)
	}

	badDataClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("550 nope\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := badDataClient.data([]byte("hello")); err == nil {
		t.Fatalf("data should fail on invalid initial status")
	}

	_ = tls.VersionTLS12
}

func TestSMTPHelloAuthStartTLSAndUpgradeBranches(t *testing.T) {
	helloClient := &smtpClient{
		reader: bufio.NewReader(strings.NewReader("550 no ehlo\r\n250 hello\r\n")),
		writer: bufio.NewWriter(bytes.NewBuffer(nil)),
	}
	if err := helloClient.hello(); err != nil {
		t.Fatalf("expected HELO fallback success, got %v", err)
	}

	failHelloClient := &smtpClient{
		reader: bufio.NewReader(strings.NewReader("550 no ehlo\r\n550 no helo\r\n")),
		writer: bufio.NewWriter(bytes.NewBuffer(nil)),
	}
	if err := failHelloClient.hello(); err == nil {
		t.Fatalf("expected hello failure when both EHLO/HELO fail")
	}

	authClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("235 ok\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := authClient.authPlain("user", "pass", "smtp.example.com"); err != nil {
		t.Fatalf("expected auth plain success, got %v", err)
	}

	authClient = &smtpClient{reader: bufio.NewReader(strings.NewReader("535 bad\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := authClient.authPlain("user", "pass", "smtp.example.com"); err == nil {
		t.Fatalf("expected auth plain rejection")
	}

	connA, connB := net.Pipe()
	defer connA.Close()
	_ = connB.Close()
	startTLSClient := &smtpClient{conn: connA, reader: bufio.NewReader(strings.NewReader("220 go\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := startTLSClient.startTLS(&tls.Config{ServerName: "localhost", InsecureSkipVerify: true}); err == nil {
		t.Fatalf("expected startTLS handshake to fail on closed peer")
	}

	m := NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityStartTLS)
	if err := m.upgradeAndAuth(&smtpClient{capabilities: nil}, "smtp.example.com", 25); err == nil {
		t.Fatalf("expected starttls unsupported error")
	}

	m = NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityNone).SetAuth("user", "pass")
	if err := m.upgradeAndAuth(&smtpClient{capabilities: nil}, "smtp.example.com", 25); err == nil {
		t.Fatalf("expected auth unsupported error")
	}

	m = NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityNone).SetAuth("user", "pass")
	if err := m.upgradeAndAuth(&smtpClient{capabilities: []string{"AUTH PLAIN"}, reader: bufio.NewReader(strings.NewReader("235 ok\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}, "smtp.example.com", 25); err != nil {
		t.Fatalf("expected upgrade/auth auth success path, got %v", err)
	}

	m = NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityAuto).SetAuth("user", "pass")
	if err := m.upgradeAndAuth(&smtpClient{conn: &tls.Conn{}, capabilities: []string{"SIZE 1"}}, "smtp.example.com", 25); err == nil || !strings.Contains(err.Error(), "smtp auth is unavailable") {
		t.Fatalf("expected auth unavailable error, got %v", err)
	}

	m = NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityAuto).SetAuth("user", "pass")
	if err := m.upgradeAndAuth(&smtpClient{capabilities: []string{"AUTH PLAIN"}}, "smtp.example.com", 25); err == nil || !strings.Contains(err.Error(), "smtp auth requires TLS") {
		t.Fatalf("expected plaintext auth rejection, got %v", err)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestSMTPValidationAndLowLevelErrorBranches(t *testing.T) {
	m := NewSMTPMailer("smtp.example.com", 25).SetSecurity(SecurityNone)

	msgNoFrom := New().AddTo("recipient@example.com").SetTextBody("b")
	if err := m.Send(msgNoFrom); err == nil || !strings.Contains(err.Error(), "invalid sender") {
		t.Fatalf("expected invalid sender error, got %v", err)
	}

	msgNoRecipients := New().SetFrom("sender@example.com").SetTextBody("b")
	if err := m.Send(msgNoRecipients); err == nil || !strings.Contains(err.Error(), "at least one recipient") {
		t.Fatalf("expected no recipients error, got %v", err)
	}

	msgBadRecipient := New().SetFrom("sender@example.com").AddTo("invalid").SetTextBody("b")
	if err := m.Send(msgBadRecipient); err == nil || !strings.Contains(err.Error(), "invalid recipient") {
		t.Fatalf("expected invalid recipient error, got %v", err)
	}

	brokenServers := &Mailer{
		host:     "smtp.example.com",
		port:     25,
		servers:  []SMTPServer{{Host: "", Port: 25}},
		security: SecurityNone,
		timeout:  time.Second,
	}
	if err := brokenServers.Send(New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetTextBody("b")); err == nil || !strings.Contains(err.Error(), "no smtp servers configured") {
		t.Fatalf("expected no smtp servers configured error, got %v", err)
	}

	client := &smtpClient{writer: bufio.NewWriter(errWriter{}), reader: bufio.NewReader(strings.NewReader("250 ok\r\n"))}
	if _, _, err := client.command("NOOP"); err == nil {
		t.Fatalf("expected command writer error")
	}

	greeting := &smtpClient{reader: bufio.NewReader(strings.NewReader("250 not-greeting\r\n"))}
	if err := greeting.readGreeting(); err == nil {
		t.Fatalf("expected non-220 greeting failure")
	}

	oneLineStatus := &smtpClient{reader: bufio.NewReader(strings.NewReader("250\r\n"))}
	if code, _, err := oneLineStatus.readResponse(); err != nil || code != 250 {
		t.Fatalf("expected 3-char status line to parse, got code=%d err=%v", code, err)
	}

	tlsMailer := NewSMTPMailer("127.0.0.1", 1).SetSecurity(SecurityTLS).SetTimeout(200 * time.Millisecond)
	if _, err := tlsMailer.newClient(SMTPServer{Host: "127.0.0.1", Port: 1}); err == nil {
		t.Fatalf("expected tls newClient dial failure")
	}

	plainMailer := NewSMTPMailer("127.0.0.1", 1).SetSecurity(SecurityNone).SetTimeout(200 * time.Millisecond)
	if _, err := plainMailer.newClient(SMTPServer{Host: "127.0.0.1", Port: 1}); err == nil {
		t.Fatalf("expected plain newClient dial failure")
	}
}

func TestSMTPMoreLowLevelBranches(t *testing.T) {
	startTLSRejected := &smtpClient{reader: bufio.NewReader(strings.NewReader("500 no\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := startTLSRejected.startTLS(&tls.Config{ServerName: "localhost", InsecureSkipVerify: true}); err == nil {
		t.Fatalf("expected startTLS rejection")
	}

	cmdClient := &smtpClient{writer: bufio.NewWriter(bytes.NewBuffer(nil)), reader: bufio.NewReader(strings.NewReader("bad\r\n"))}
	if _, _, err := cmdClient.command("NOOP"); err == nil {
		t.Fatalf("expected command read-response error")
	}

	greetingClient := &smtpClient{reader: bufio.NewReader(errReader{})}
	if err := greetingClient.readGreeting(); err == nil {
		t.Fatalf("expected greeting read error")
	}

	dataRejectClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("354 go\r\n550 rejected\r\n")), writer: bufio.NewWriter(bytes.NewBuffer(nil))}
	if err := dataRejectClient.data([]byte("hello")); err == nil {
		t.Fatalf("expected final data rejection")
	}

	dataWriteErrClient := &smtpClient{reader: bufio.NewReader(strings.NewReader("354 go\r\n")), writer: bufio.NewWriter(errWriter{})}
	if err := dataWriteErrClient.data([]byte("hello")); err == nil {
		t.Fatalf("expected data write error")
	}

	errCommandClient := &smtpClient{writer: bufio.NewWriter(errWriter{}), reader: bufio.NewReader(strings.NewReader("250 ok\r\n"))}
	if err := errCommandClient.mail("sender@example.com"); err == nil {
		t.Fatalf("expected mail command error")
	}
	errCommandClient = &smtpClient{writer: bufio.NewWriter(errWriter{}), reader: bufio.NewReader(strings.NewReader("250 ok\r\n"))}
	if err := errCommandClient.rcpt("recipient@example.com"); err == nil {
		t.Fatalf("expected rcpt command error")
	}
	errCommandClient = &smtpClient{writer: bufio.NewWriter(errWriter{}), reader: bufio.NewReader(strings.NewReader("221 bye\r\n"))}
	if err := errCommandClient.quit(); err == nil {
		t.Fatalf("expected quit command error")
	}
	errCommandClient = &smtpClient{writer: bufio.NewWriter(errWriter{}), reader: bufio.NewReader(strings.NewReader("235 ok\r\n"))}
	if err := errCommandClient.authPlain("user", "pass", "smtp.example.com"); err == nil {
		t.Fatalf("expected auth command error")
	}
}

func TestSMTPMailerInterceptorAndDKIMFailures(t *testing.T) {
	server := startSMTPTestServer(t, false)
	defer server.close()
	host, port := server.hostPort(t)

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")

	interceptFail := NewSMTPMailer(host, port).
		SetSecurity(SecurityNone).
		Use(func(m *Message) error { return errors.New("interceptor boom") })
	if err := interceptFail.Send(msg); err == nil || !strings.Contains(err.Error(), "interceptor boom") {
		t.Fatalf("expected interceptor failure, got %v", err)
	}

	badDKIM := NewSMTPMailer(host, port).
		SetSecurity(SecurityNone).
		SetDKIMSigner(&DKIMSigner{})
	if err := badDKIM.Send(msg); err == nil || !strings.Contains(err.Error(), "dkim signing failed") {
		t.Fatalf("expected dkim failure, got %v", err)
	}
}

func TestSMTPMailerGreetingRCPTDataAndQuitFailures(t *testing.T) {
	message := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("s").SetTextBody("b")

	greetingServer := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.greeting = "500 bad greeting"
	})
	defer greetingServer.close()
	host, port := greetingServer.hostPort(t)
	mailer := NewSMTPMailer(host, port).SetSecurity(SecurityNone).SetTimeout(2 * time.Second)
	if err := mailer.Send(message); err == nil || !strings.Contains(err.Error(), "unexpected greeting status") {
		t.Fatalf("expected greeting failure, got %v", err)
	}

	helloServer := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.ehloCode = "550 no ehlo"
		s.heloCode = "550 no helo"
	})
	defer helloServer.close()
	host, port = helloServer.hostPort(t)
	mailer = NewSMTPMailer(host, port).SetSecurity(SecurityNone).SetTimeout(2 * time.Second)
	if err := mailer.Send(message); err == nil || !strings.Contains(err.Error(), "ehlo/helo failed") {
		t.Fatalf("expected hello failure, got %v", err)
	}

	rcptServer := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.rcptCode = "550 no recipient"
	})
	defer rcptServer.close()
	host, port = rcptServer.hostPort(t)
	mailer = NewSMTPMailer(host, port).SetSecurity(SecurityNone).SetTimeout(2 * time.Second)
	if err := mailer.Send(message); err == nil || !strings.Contains(err.Error(), "smtp rcpt to") {
		t.Fatalf("expected rcpt failure, got %v", err)
	}

	dataServer := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.dataCode = "550 not queued"
	})
	defer dataServer.close()
	host, port = dataServer.hostPort(t)
	mailer = NewSMTPMailer(host, port).SetSecurity(SecurityNone).SetTimeout(2 * time.Second)
	if err := mailer.Send(message); err == nil || !strings.Contains(err.Error(), "smtp data command failed") {
		t.Fatalf("expected data failure, got %v", err)
	}

	quitServer := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.quitCode = "250 quit denied"
	})
	defer quitServer.close()
	host, port = quitServer.hostPort(t)
	mailer = NewSMTPMailer(host, port).SetSecurity(SecurityNone).SetTimeout(2 * time.Second)
	if err := mailer.Send(message); err == nil || !strings.Contains(err.Error(), "smtp quit failed") {
		t.Fatalf("expected quit failure, got %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

type smtpTLSTestServer struct {
	address     string
	listener    net.Listener
	startTLS    bool
	requireAuth bool
	username    string
	password    string

	mu          sync.Mutex
	commands    []string
	messageData string
}

func startSMTPTLSTestServer(t *testing.T, startTLS bool, requireAuth bool) *smtpTLSTestServer {
	t.Helper()
	certificate := mustGenerateServerCertificate(t)

	var (
		ln  net.Listener
		err error
	)
	if startTLS {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	} else {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}})
	}
	if err != nil {
		t.Fatalf("failed to start tls smtp listener: %v", err)
	}

	server := &smtpTLSTestServer{
		address:     ln.Addr().String(),
		listener:    ln,
		startTLS:    startTLS,
		requireAuth: requireAuth,
		username:    "user",
		password:    "pass",
	}

	go server.serve(t, certificate)
	return server
}

func (s *smtpTLSTestServer) close() {
	_ = s.listener.Close()
}

func (s *smtpTLSTestServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portRaw, err := net.SplitHostPort(s.address)
	if err != nil {
		t.Fatalf("split host/port failed: %v", err)
	}

	var port int
	if _, err = fmt.Sscanf(portRaw, "%d", &port); err != nil {
		t.Fatalf("parse port failed: %v", err)
	}

	return host, port
}

func (s *smtpTLSTestServer) serve(t *testing.T, certificate tls.Certificate) {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	_, _ = w.WriteString("220 localhost ESMTP ready\r\n")
	_ = w.Flush()

	tlsActive := !s.startTLS
	authenticated := false

	for {
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		s.recordCommand(line)

		switch {
		case strings.HasPrefix(line, "EHLO "):
			_, _ = w.WriteString("250-localhost\r\n")
			if s.startTLS && !tlsActive {
				_, _ = w.WriteString("250-STARTTLS\r\n")
			}
			if s.requireAuth {
				_, _ = w.WriteString("250-AUTH PLAIN\r\n")
			}
			_, _ = w.WriteString("250 SIZE 10485760\r\n")
			_ = w.Flush()
		case line == "STARTTLS":
			_, _ = w.WriteString("220 ready for tls\r\n")
			_ = w.Flush()

			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			r = bufio.NewReader(tlsConn)
			w = bufio.NewWriter(tlsConn)
			tlsActive = true
		case strings.HasPrefix(line, "AUTH PLAIN "):
			payload := strings.TrimPrefix(line, "AUTH PLAIN ")
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				_, _ = w.WriteString("535 bad auth\r\n")
				_ = w.Flush()
				continue
			}
			expected := "\x00" + s.username + "\x00" + s.password
			if string(decoded) != expected {
				_, _ = w.WriteString("535 bad auth\r\n")
			} else {
				authenticated = true
				_, _ = w.WriteString("235 ok\r\n")
			}
			_ = w.Flush()
		case strings.HasPrefix(line, "MAIL FROM:"):
			if s.requireAuth && !authenticated {
				_, _ = w.WriteString("530 auth required\r\n")
			} else {
				_, _ = w.WriteString("250 ok\r\n")
			}
			_ = w.Flush()
		case strings.HasPrefix(line, "RCPT TO:"):
			_, _ = w.WriteString("250 ok\r\n")
			_ = w.Flush()
		case line == "DATA":
			_, _ = w.WriteString("354 end with <CR><LF>.<CR><LF>\r\n")
			_ = w.Flush()
			var dataLines []string
			for {
				dataLine, dataErr := r.ReadString('\n')
				if dataErr != nil {
					return
				}
				dataLine = strings.TrimRight(dataLine, "\r\n")
				if dataLine == "." {
					break
				}
				if strings.HasPrefix(dataLine, "..") {
					dataLine = dataLine[1:]
				}
				dataLines = append(dataLines, dataLine)
			}
			s.recordMessage(strings.Join(dataLines, "\n"))
			_, _ = w.WriteString("250 queued\r\n")
			_ = w.Flush()
		case line == "QUIT":
			_, _ = w.WriteString("221 bye\r\n")
			_ = w.Flush()
			return
		default:
			_, _ = w.WriteString("250 ok\r\n")
			_ = w.Flush()
		}
	}
}

func (s *smtpTLSTestServer) recordCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, command)
}

func (s *smtpTLSTestServer) recordMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageData = message
}

func (s *smtpTLSTestServer) hasCommand(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func mustGenerateServerCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate failed: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build tls key pair failed: %v", err)
	}

	return certificate
}

func TestSMTPMailerImplicitTLSAndStartTLSSuccess(t *testing.T) {
	tlsServer := startSMTPTLSTestServer(t, false, false)
	defer tlsServer.close()
	host, port := tlsServer.hostPort(t)

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("tls").SetTextBody("body")
	mailer := NewSMTPMailer(host, port).
		SetSecurity(SecurityTLS).
		SetInsecureSkipVerify(true).
		SetTimeout(2 * time.Second)
	if err := mailer.Send(msg); err != nil {
		t.Fatalf("implicit tls send failed: %v", err)
	}

	if !tlsServer.hasCommand("MAIL FROM:") {
		t.Fatalf("expected implicit tls session to reach MAIL FROM")
	}

	startTLSServer := startSMTPTLSTestServer(t, true, true)
	defer startTLSServer.close()
	host, port = startTLSServer.hostPort(t)

	mailer = NewSMTPMailer(host, port).
		SetSecurity(SecurityStartTLS).
		SetInsecureSkipVerify(true).
		SetAuth("user", "pass").
		SetTimeout(2 * time.Second)
	if err := mailer.Send(msg); err != nil {
		t.Fatalf("starttls send failed: %v", err)
	}

	if !startTLSServer.hasCommand("STARTTLS") || !startTLSServer.hasCommand("AUTH PLAIN ") {
		t.Fatalf("expected STARTTLS and AUTH commands in upgraded session")
	}
}

func TestSMTPMailerRejectsPlaintextAuthInAutoMode(t *testing.T) {
	server := startSMTPScenarioServer(t, func(s *smtpTestServer) {
		s.ehloCode = "250-localhost\r\n250-AUTH PLAIN\r\n250 SIZE 10485760"
	})
	defer server.close()

	host, port := server.hostPort(t)
	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("plain auth").
		SetTextBody("body")

	mailer := NewSMTPMailer(host, port).
		SetSecurity(SecurityAuto).
		SetAuth("user", "pass").
		SetTimeout(2 * time.Second)

	err := mailer.Send(message)
	if err == nil || !strings.Contains(err.Error(), "smtp auth requires TLS") {
		t.Fatalf("expected plaintext auth rejection, got %v", err)
	}

	if server.hasCommand("AUTH PLAIN ") {
		t.Fatalf("expected client to reject plaintext auth before sending credentials")
	}
}
