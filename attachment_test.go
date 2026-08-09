package mailer

import "testing"

func TestNewAttachmentClonesDataAndDetectsType(t *testing.T) {
	input := []byte("hello")
	a := NewAttachment("readme.txt", input)

	if a.ContentType == "" {
		t.Fatalf("expected content type")
	}

	input[0] = 'X'
	if string(a.Data) != "hello" {
		t.Fatalf("expected attachment data clone, got: %q", string(a.Data))
	}
}

func TestNewInlineAttachmentAndUnknownTypeFallback(t *testing.T) {
	a := NewInlineAttachment("file.unknownext", []byte("x"), "cid-1")

	if !a.Inline {
		t.Fatalf("expected inline attachment")
	}

	if a.ContentID != "cid-1" {
		t.Fatalf("unexpected content id: %q", a.ContentID)
	}

	if a.ContentType != "application/octet-stream" {
		t.Fatalf("expected fallback content type, got: %q", a.ContentType)
	}
}
