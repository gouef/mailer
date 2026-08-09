package mailer

import "testing"

func TestAddressStringAndParse(t *testing.T) {
	address, err := NewAddress("john@example.com", "John Doe")
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}

	formatted, err := address.String()
	if err != nil {
		t.Fatalf("Address.String failed: %v", err)
	}

	if formatted != `"John Doe" <john@example.com>` {
		t.Fatalf("unexpected formatted mailbox: %s", formatted)
	}

	parsed, err := ParseAddressValue(formatted)
	if err != nil {
		t.Fatalf("ParseAddressValue failed: %v", err)
	}

	if parsed.Email != "john@example.com" || parsed.Name != "John Doe" {
		t.Fatalf("unexpected parsed value: %+v", parsed)
	}
}

func TestMessageMailboxMethods(t *testing.T) {
	from, err := NewAddress("sender@example.com", "Sender")
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}
	to, err := NewAddress("recipient@example.com", "Recipient")
	if err != nil {
		t.Fatalf("NewAddress failed: %v", err)
	}

	m := New()
	if _, err = m.SetFromMailbox(from); err != nil {
		t.Fatalf("SetFromMailbox failed: %v", err)
	}
	if _, err = m.AddToMailbox(to); err != nil {
		t.Fatalf("AddToMailbox failed: %v", err)
	}

	if len(m.GetFrom()) != 1 || m.GetFrom()[0] != `"Sender" <sender@example.com>` {
		t.Fatalf("unexpected from mailbox: %v", m.GetFrom())
	}

	if len(m.GetTo()) != 1 || m.GetTo()[0] != `"Recipient" <recipient@example.com>` {
		t.Fatalf("unexpected to mailbox: %v", m.GetTo())
	}
}

func TestMessageMailboxCollectionMethods(t *testing.T) {
	from, _ := NewAddress("sender@example.com", "Sender")
	to1, _ := NewAddress("one@example.com", "One")
	to2, _ := NewAddress("two@example.com", "Two")
	cc1, _ := NewAddress("cc1@example.com", "CC One")
	cc2, _ := NewAddress("cc2@example.com", "CC Two")

	m := New()
	if _, err := m.SetFromMailbox(from); err != nil {
		t.Fatalf("SetFromMailbox failed: %v", err)
	}
	if _, err := m.SetToMailboxes([]Address{to1}); err != nil {
		t.Fatalf("SetToMailboxes failed: %v", err)
	}
	if _, err := m.AddToMailboxes(to2); err != nil {
		t.Fatalf("AddToMailboxes failed: %v", err)
	}
	if _, err := m.SetCcMailboxes([]Address{cc1}); err != nil {
		t.Fatalf("SetCcMailboxes failed: %v", err)
	}
	if _, err := m.AddCcMailboxes(cc2); err != nil {
		t.Fatalf("AddCcMailboxes failed: %v", err)
	}

	if len(m.GetTo()) != 2 {
		t.Fatalf("expected 2 to recipients, got %v", m.GetTo())
	}

	if len(m.GetCc()) != 2 {
		t.Fatalf("expected 2 cc recipients, got %v", m.GetCc())
	}
}

func TestMessageMailboxCollectionMethodsRejectInvalid(t *testing.T) {
	valid, _ := NewAddress("ok@example.com", "Ok")
	invalid := Address{Email: "invalid", Name: "Broken"}

	m := New()
	if _, err := m.SetToMailboxes([]Address{valid}); err != nil {
		t.Fatalf("SetToMailboxes failed: %v", err)
	}

	if _, err := m.AddToMailboxes(invalid); err == nil {
		t.Fatalf("expected AddToMailboxes error")
	}

	if len(m.GetTo()) != 1 {
		t.Fatalf("expected previous to list unchanged, got %v", m.GetTo())
	}
}

func TestSetFromMailboxesRequiresExactlyOne(t *testing.T) {
	m := New()
	one, _ := NewAddress("one@example.com", "One")
	two, _ := NewAddress("two@example.com", "Two")

	if _, err := m.SetFromMailboxes(nil); err == nil {
		t.Fatalf("expected error for zero from mailboxes")
	}

	if _, err := m.SetFromMailboxes([]Address{one, two}); err == nil {
		t.Fatalf("expected error for multiple from mailboxes")
	}

	if _, err := m.SetFromMailboxes([]Address{one}); err != nil {
		t.Fatalf("expected success for single from mailbox: %v", err)
	}
}

func TestParseAddressValueRejectsInvalidInput(t *testing.T) {
	if _, err := ParseAddressValue("invalid-address"); err == nil {
		t.Fatalf("expected parse error for invalid address value")
	}
}

func TestAddressMustStringPanicsOnInvalidAddress(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic from MustString for invalid address")
		}
	}()

	_ = (Address{Email: "invalid", Name: "Broken"}).MustString()
}

func TestAddressMustStringSuccess(t *testing.T) {
	value := (Address{Email: "john@example.com", Name: "John"}).MustString()
	if value == "" {
		t.Fatalf("expected non-empty formatted mailbox")
	}
}
