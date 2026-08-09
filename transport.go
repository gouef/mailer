package mailer

import (
	"errors"
	"fmt"
	"strings"
)

type Sender interface {
	Send(message *Message) error
}

type Interceptor func(message *Message) error

type FallbackMailer struct {
	mailers      []Sender
	interceptors []Interceptor
}

func NewFallbackMailer(mailers ...Sender) *FallbackMailer {
	cloned := make([]Sender, len(mailers))
	copy(cloned, mailers)

	return &FallbackMailer{mailers: cloned}
}

func (m *FallbackMailer) AddMailer(mailer Sender) *FallbackMailer {
	m.mailers = append(m.mailers, mailer)
	return m
}

func (m *FallbackMailer) Use(interceptor Interceptor) *FallbackMailer {
	if interceptor != nil {
		m.interceptors = append(m.interceptors, interceptor)
	}
	return m
}

func (m *FallbackMailer) Send(message *Message) error {
	if message == nil {
		return errors.New("message is nil")
	}

	if len(m.mailers) == 0 {
		return errors.New("no mailers configured")
	}

	base := message.Clone()
	if err := applyInterceptors(base, m.interceptors); err != nil {
		return err
	}

	failures := make([]string, 0, len(m.mailers))
	for i, mailer := range m.mailers {
		if mailer == nil {
			continue
		}

		if err := mailer.Send(base.Clone()); err == nil {
			return nil
		} else {
			failures = append(failures, fmt.Sprintf("mailer#%d: %v", i+1, err))
		}
	}

	if len(failures) == 0 {
		return errors.New("no valid mailers configured")
	}

	return fmt.Errorf("all fallback mailers failed: %s", strings.Join(failures, " | "))
}

func applyInterceptors(message *Message, interceptors []Interceptor) error {
	for _, interceptor := range interceptors {
		if interceptor == nil {
			continue
		}

		if err := interceptor(message); err != nil {
			return err
		}
	}

	return nil
}
