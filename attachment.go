package mailer

import "mime"

type Attachment struct {
	Name        string
	Data        []byte
	ContentType string
	Inline      bool
	ContentID   string
}

func NewAttachment(name string, data []byte) Attachment {
	return Attachment{
		Name:        name,
		Data:        cloneBytes(data),
		ContentType: detectContentType(name),
	}
}

func NewInlineAttachment(name string, data []byte, contentID string) Attachment {
	a := NewAttachment(name, data)
	a.Inline = true
	a.ContentID = contentID
	return a
}

func detectContentType(name string) string {
	if ext := mime.TypeByExtension(getFileExt(name)); ext != "" {
		return ext
	}

	return "application/octet-stream"
}
