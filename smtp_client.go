package mailer

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type smtpClient struct {
	conn         net.Conn
	reader       *bufio.Reader
	writer       *bufio.Writer
	host         string
	capabilities []string
}

func newSMTPClient(conn net.Conn, host string) *smtpClient {
	return &smtpClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		host:   host,
	}
}

func (c *smtpClient) close() {
	_ = c.conn.Close()
}

func (c *smtpClient) hello() error {
	localHost := "localhost"
	code, lines, err := c.command(fmt.Sprintf("EHLO %s", localHost))
	if err != nil {
		return err
	}
	if code != 250 {
		code, _, heloErr := c.command(fmt.Sprintf("HELO %s", localHost))
		if heloErr != nil {
			return heloErr
		}
		if code != 250 {
			return fmt.Errorf("ehlo/helo failed with status %d", code)
		}
		c.capabilities = nil
		return nil
	}

	capabilities := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		capabilities = append(capabilities, strings.ToUpper(trimmed))
	}
	if len(capabilities) > 0 {
		c.capabilities = capabilities[1:]
	}

	return nil
}

func (c *smtpClient) readGreeting() error {
	code, _, err := c.readResponse()
	if err != nil {
		return err
	}
	if code != 220 {
		return fmt.Errorf("unexpected greeting status: %d", code)
	}
	return nil
}

func (c *smtpClient) hasCapability(name string) bool {
	needle := strings.ToUpper(strings.TrimSpace(name))
	for _, capability := range c.capabilities {
		if capability == needle || strings.HasPrefix(capability, needle+" ") {
			return true
		}
	}
	return false
}

func (c *smtpClient) isTLS() bool {
	_, ok := c.conn.(*tls.Conn)
	return ok
}

func (c *smtpClient) startTLS(config *tls.Config) error {
	code, _, err := c.command("STARTTLS")
	if err != nil {
		return err
	}
	if code != 220 {
		return fmt.Errorf("starttls rejected with status %d", code)
	}

	tlsConn := tls.Client(c.conn, config)
	if err := tlsConn.Handshake(); err != nil {
		return err
	}

	c.conn = tlsConn
	c.reader = bufio.NewReader(tlsConn)
	c.writer = bufio.NewWriter(tlsConn)
	c.capabilities = nil
	return nil
}

func (c *smtpClient) authPlain(username string, password string, host string) error {
	_ = host
	credentials := "\x00" + username + "\x00" + password
	payload := base64.StdEncoding.EncodeToString([]byte(credentials))
	code, _, err := c.command("AUTH PLAIN " + payload)
	if err != nil {
		return err
	}
	if code != 235 {
		return fmt.Errorf("auth rejected with status %d", code)
	}
	return nil
}

func (c *smtpClient) mail(from string) error {
	code, _, err := c.command("MAIL FROM:<" + from + ">")
	if err != nil {
		return err
	}
	if code != 250 {
		return fmt.Errorf("mail from rejected with status %d", code)
	}
	return nil
}

func (c *smtpClient) rcpt(to string) error {
	code, _, err := c.command("RCPT TO:<" + to + ">")
	if err != nil {
		return err
	}
	if code != 250 && code != 251 {
		return fmt.Errorf("rcpt rejected with status %d", code)
	}
	return nil
}

func (c *smtpClient) data(message []byte) error {
	code, _, err := c.command("DATA")
	if err != nil {
		return err
	}
	if code != 354 {
		return fmt.Errorf("data rejected with status %d", code)
	}

	if err := c.writeMessageData(message); err != nil {
		return err
	}

	code, _, err = c.readResponse()
	if err != nil {
		return err
	}
	if code != 250 {
		return fmt.Errorf("message not accepted, status %d", code)
	}

	return nil
}

func (c *smtpClient) writeMessageData(message []byte) error {
	text := strings.ReplaceAll(string(message), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		if _, err := c.writer.WriteString(line + "\r\n"); err != nil {
			return err
		}
	}

	if _, err := c.writer.WriteString(".\r\n"); err != nil {
		return err
	}

	return c.writer.Flush()
}

func (c *smtpClient) quit() error {
	code, _, err := c.command("QUIT")
	if err != nil {
		return err
	}
	if code != 221 {
		return fmt.Errorf("quit rejected with status %d", code)
	}
	return nil
}

func (c *smtpClient) command(command string) (int, []string, error) {
	if _, err := c.writer.WriteString(command + "\r\n"); err != nil {
		return 0, nil, err
	}
	if err := c.writer.Flush(); err != nil {
		return 0, nil, err
	}

	return c.readResponse()
}

func (c *smtpClient) readResponse() (int, []string, error) {
	lines := make([]string, 0, 2)
	status := 0

	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return 0, nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if len(line) < 3 {
			return 0, nil, fmt.Errorf("invalid smtp response %q", line)
		}

		currentStatus, err := strconv.Atoi(line[:3])
		if err != nil {
			return 0, nil, fmt.Errorf("invalid smtp status %q", line[:3])
		}

		if status == 0 {
			status = currentStatus
		}

		payload := ""
		if len(line) > 4 {
			payload = line[4:]
		}
		lines = append(lines, payload)

		if len(line) >= 4 && line[3] == '-' {
			continue
		}

		if len(line) >= 4 && line[3] == ' ' {
			break
		}

		if len(line) == 3 {
			break
		}
	}

	return status, lines, nil
}
