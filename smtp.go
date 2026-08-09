package mailer

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type SecurityMode string

const (
	SecurityAuto     SecurityMode = "auto"
	SecurityNone     SecurityMode = "none"
	SecurityStartTLS SecurityMode = "starttls"
	SecurityTLS      SecurityMode = "tls"
)

type Mailer struct {
	host               string
	port               int
	servers            []SMTPServer
	username           string
	password           string
	security           SecurityMode
	insecureSkipVerify bool
	timeout            time.Duration
	interceptors       []Interceptor
	dkimSigner         *DKIMSigner
}

type SMTPServer struct {
	Host string
	Port int
}

func NewSMTPMailer(host string, port int) *Mailer {
	return &Mailer{
		host:     strings.TrimSpace(host),
		port:     port,
		servers:  []SMTPServer{{Host: strings.TrimSpace(host), Port: port}},
		security: SecurityAuto,
		timeout:  10 * time.Second,
	}
}

func (m *Mailer) AddServer(host string, port int) *Mailer {
	m.servers = append(m.servers, SMTPServer{Host: strings.TrimSpace(host), Port: port})
	return m
}

func (m *Mailer) SetServers(servers []SMTPServer) *Mailer {
	m.servers = append([]SMTPServer(nil), servers...)
	if len(m.servers) > 0 {
		m.host = strings.TrimSpace(m.servers[0].Host)
		m.port = m.servers[0].Port
	}
	return m
}

func (m *Mailer) SetAuth(username string, password string) *Mailer {
	m.username = strings.TrimSpace(username)
	m.password = password
	return m
}

func (m *Mailer) SetSecurity(mode SecurityMode) *Mailer {
	m.security = mode
	return m
}

func (m *Mailer) SetInsecureSkipVerify(skip bool) *Mailer {
	m.insecureSkipVerify = skip
	return m
}

func (m *Mailer) SetTimeout(timeout time.Duration) *Mailer {
	if timeout > 0 {
		m.timeout = timeout
	}
	return m
}

func (m *Mailer) Use(interceptor Interceptor) *Mailer {
	if interceptor != nil {
		m.interceptors = append(m.interceptors, interceptor)
	}
	return m
}

func (m *Mailer) SetDKIMSigner(signer *DKIMSigner) *Mailer {
	m.dkimSigner = signer
	return m
}

func (m *Mailer) Send(message *Message) error {
	if message == nil {
		return errors.New("message is nil")
	}

	working := message.Clone()
	if err := applyInterceptors(working, m.interceptors); err != nil {
		return err
	}

	if m.host == "" {
		return errors.New("smtp host is required")
	}

	if m.port <= 0 {
		return errors.New("smtp port must be greater than 0")
	}

	fromAddress, err := working.envelopeFrom()
	if err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}

	rawRecipients := working.Recipients()
	if len(rawRecipients) == 0 {
		return errors.New("message must contain at least one recipient")
	}

	recipients, err := normalizeRecipients(rawRecipients)
	if err != nil {
		return err
	}

	mimeMessage, err := working.ToMIME()
	if err != nil {
		return fmt.Errorf("build mime message: %w", err)
	}

	if m.dkimSigner != nil {
		mimeMessage, err = m.dkimSigner.Sign(mimeMessage)
		if err != nil {
			return fmt.Errorf("dkim signing failed: %w", err)
		}
	}

	servers := m.getServers()
	if len(servers) == 0 {
		return errors.New("no smtp servers configured")
	}

	failures := make([]string, 0)
	for _, server := range servers {
		if strings.TrimSpace(server.Host) == "" || server.Port <= 0 {
			continue
		}

		if err = m.sendViaServer(server, fromAddress, recipients, mimeMessage); err == nil {
			return nil
		}

		failures = append(failures, fmt.Sprintf("%s:%d: %v", server.Host, server.Port, err))
	}

	if len(failures) == 0 {
		return errors.New("no valid smtp servers configured")
	}

	return fmt.Errorf("all smtp servers failed: %s", strings.Join(failures, " | "))
}

func (m *Mailer) sendViaServer(server SMTPServer, fromAddress string, recipients []string, mimeMessage []byte) error {
	client, err := m.newClient(server)
	if err != nil {
		return err
	}
	defer client.close()

	if err = client.hello(); err != nil {
		return err
	}

	if err = m.upgradeAndAuth(client, server.Host, server.Port); err != nil {
		_ = client.quit()
		return err
	}

	if err = client.mail(fromAddress); err != nil {
		_ = client.quit()
		return fmt.Errorf("smtp mail from failed: %w", err)
	}

	for _, recipient := range recipients {
		if err = client.rcpt(recipient); err != nil {
			_ = client.quit()
			return fmt.Errorf("smtp rcpt to %q failed: %w", recipient, err)
		}
	}

	if err = client.data(mimeMessage); err != nil {
		_ = client.quit()
		return fmt.Errorf("smtp data command failed: %w", err)
	}

	if err = client.quit(); err != nil {
		return fmt.Errorf("smtp quit failed: %w", err)
	}

	return nil
}

func (m *Mailer) newClient(server SMTPServer) (*smtpClient, error) {
	address := net.JoinHostPort(server.Host, strconv.Itoa(server.Port))
	dialer := &net.Dialer{Timeout: m.timeout}

	if m.effectiveSecurity(server.Port) == SecurityTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", address, m.tlsConfig(server.Host))
		if err != nil {
			return nil, fmt.Errorf("connect with tls failed: %w", err)
		}

		client := newSMTPClient(conn, server.Host)
		if err := client.readGreeting(); err != nil {
			client.close()
			return nil, err
		}

		return client, nil
	}

	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect smtp server failed: %w", err)
	}

	client := newSMTPClient(conn, server.Host)
	if err := client.readGreeting(); err != nil {
		client.close()
		return nil, err
	}

	return client, nil
}

func (m *Mailer) upgradeAndAuth(client *smtpClient, host string, port int) error {
	security := m.effectiveSecurity(port)
	if security == SecurityStartTLS || security == SecurityAuto {
		if !client.hasCapability("STARTTLS") {
			if security == SecurityStartTLS {
				return errors.New("smtp server does not support STARTTLS")
			}
		} else {
			if err := client.startTLS(m.tlsConfig(host)); err != nil {
				return fmt.Errorf("starttls failed: %w", err)
			}

			if err := client.hello(); err != nil {
				return err
			}
		}
	}

	if security == SecurityStartTLS && !client.isTLS() {
		return errors.New("starttls requested but tls was not established")
	}

	if m.username == "" {
		return nil
	}

	if !client.isTLS() && security != SecurityNone {
		return errors.New("smtp auth requires TLS; use SecurityNone to allow plaintext auth")
	}

	if !client.hasCapability("AUTH") {
		if security == SecurityNone {
			return errors.New("smtp server does not support auth")
		}
		return errors.New("smtp auth is unavailable")
	}

	if err := client.authPlain(m.username, m.password, host); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}

	return nil
}

func (m *Mailer) effectiveSecurity(port int) SecurityMode {
	if m.security == SecurityTLS || m.security == SecurityStartTLS || m.security == SecurityNone {
		return m.security
	}

	if port == 465 {
		return SecurityTLS
	}

	return SecurityAuto
}
