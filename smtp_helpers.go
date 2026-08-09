package mailer

import (
	"crypto/tls"
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

func (m *Mailer) getServers() []SMTPServer {
	servers := append([]SMTPServer(nil), m.servers...)
	if len(servers) == 0 && strings.TrimSpace(m.host) != "" && m.port > 0 {
		servers = append(servers, SMTPServer{Host: strings.TrimSpace(m.host), Port: m.port})
	}

	if len(servers) == 0 {
		return nil
	}

	unique := make([]SMTPServer, 0, len(servers))
	for _, server := range servers {
		host := strings.TrimSpace(server.Host)
		if host == "" || server.Port <= 0 {
			continue
		}

		candidate := SMTPServer{Host: host, Port: server.Port}
		if slices.Contains(unique, candidate) {
			continue
		}

		unique = append(unique, candidate)
	}

	return unique
}

func parseAddress(value string) (string, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}

	return parsed.Address, nil
}

func normalizeRecipients(input []string) ([]string, error) {
	seen := make(map[string]struct{}, len(input))
	result := make([]string, 0, len(input))

	for _, value := range input {
		address, err := parseAddress(value)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient %q: %w", value, err)
		}

		if _, exists := seen[address]; exists {
			continue
		}

		seen[address] = struct{}{}
		result = append(result, address)
	}

	return result, nil
}

func (m *Mailer) tlsConfig(host string) *tls.Config {
	return &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: m.insecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}
}
