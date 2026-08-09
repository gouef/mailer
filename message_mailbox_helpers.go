package mailer

import (
	"net/mail"
	"strings"
)

func formatMailbox(address string, name string) (string, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return "", err
	}

	return mailbox.String()
}

func formatMailboxList(mailboxes []Address) ([]string, error) {
	formatted := make([]string, 0, len(mailboxes))
	for _, mailbox := range mailboxes {
		value, err := mailbox.String()
		if err != nil {
			return nil, err
		}
		formatted = append(formatted, value)
	}

	return formatted, nil
}

func formatAddressHeaderValues(values []string) string {
	formatted := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		parsed, err := mail.ParseAddress(trimmed)
		if err != nil {
			continue
		}

		formatted = append(formatted, parsed.String())
	}

	return strings.Join(formatted, ", ")
}
