package mailer

import (
	"errors"
	"strings"
	"testing"
)

type testSender struct {
	err error
	got *Message
}

func (s *testSender) Send(message *Message) error {
	s.got = message.Clone()
	return s.err
}

func TestFallbackMailerUsesNextSender(t *testing.T) {
	first := &testSender{err: errors.New("primary failed")}
	second := &testSender{}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("before")

	fallback := NewFallbackMailer(first, second).Use(func(m *Message) error {
		m.SetSubject("after")
		return nil
	})

	if err := fallback.Send(message); err != nil {
		t.Fatalf("expected fallback send success, got %v", err)
	}

	if first.got == nil || first.got.Subject != "after" {
		t.Fatalf("expected interceptor to run before first sender")
	}

	if second.got == nil || second.got.Subject != "after" {
		t.Fatalf("expected interceptor to run before second sender")
	}

	if message.Subject != "before" {
		t.Fatalf("original message should stay unchanged")
	}
}

func TestFallbackMailerReturnsAggregateError(t *testing.T) {
	fallback := NewFallbackMailer(
		&testSender{err: errors.New("first down")},
		&testSender{err: errors.New("second down")},
	)

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com")
	err := fallback.Send(msg)
	if err == nil {
		t.Fatalf("expected error")
	}

	if !strings.Contains(err.Error(), "first down") || !strings.Contains(err.Error(), "second down") {
		t.Fatalf("expected aggregated errors, got %v", err)
	}
}

func TestFallbackMailerConfigurationAndErrors(t *testing.T) {
	fallback := NewFallbackMailer()

	if err := fallback.Send(nil); err == nil || !strings.Contains(err.Error(), "message is nil") {
		t.Fatalf("expected nil message error, got %v", err)
	}

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com")
	if err := fallback.Send(msg); err == nil || !strings.Contains(err.Error(), "no mailers configured") {
		t.Fatalf("expected no mailers configured error, got %v", err)
	}

	fallback.AddMailer(nil)
	if err := fallback.Send(msg); err == nil || !strings.Contains(err.Error(), "no valid mailers configured") {
		t.Fatalf("expected no valid mailers configured error, got %v", err)
	}
}

func TestFallbackMailerInterceptorErrorStopsSend(t *testing.T) {
	sender := &testSender{}
	fallback := NewFallbackMailer(sender).Use(func(m *Message) error {
		return errors.New("interceptor failed")
	})

	msg := New().SetFrom("sender@example.com").AddTo("recipient@example.com")
	err := fallback.Send(msg)
	if err == nil || !strings.Contains(err.Error(), "interceptor failed") {
		t.Fatalf("expected interceptor error, got %v", err)
	}

	if sender.got != nil {
		t.Fatalf("sender should not be called when interceptor fails")
	}
}

func TestApplyInterceptorsHandlesNilAndSuccess(t *testing.T) {
	msg := New().SetSubject("before")
	err := applyInterceptors(msg, []Interceptor{
		nil,
		func(m *Message) error {
			m.SetSubject("after")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("applyInterceptors failed: %v", err)
	}
	if msg.Subject != "after" {
		t.Fatalf("expected interceptor mutation to be applied")
	}
}
