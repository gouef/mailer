package mailer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
)

type mimeMultipartWriter interface {
	CreatePart(header textproto.MIMEHeader) (io.Writer, error)
	Close() error
	Boundary() string
}

type stdMultipartWriter struct {
	writer *multipart.Writer
}

func (w *stdMultipartWriter) CreatePart(header textproto.MIMEHeader) (io.Writer, error) {
	return w.writer.CreatePart(header)
}

func (w *stdMultipartWriter) Close() error {
	return w.writer.Close()
}

func (w *stdMultipartWriter) Boundary() string {
	return w.writer.Boundary()
}

var newMultipartWriter = func(buf *bytes.Buffer) mimeMultipartWriter {
	return &stdMultipartWriter{writer: multipart.NewWriter(buf)}
}

var encodeTextBodyForMIME = encodeTextBody
var buildAlternativePartForBodyEntity = buildAlternativePart

func buildBodyEntity(textBody string, htmlBody string, hasAlternative bool, inlineAttachments []Attachment) ([]byte, string, error) {
	if len(inlineAttachments) == 0 {
		if hasAlternative {
			altBuf, altBoundary, err := buildAlternativePartForBodyEntity(textBody, htmlBody)
			if err != nil {
				return nil, "", err
			}

			return altBuf.Bytes(), mediaType("multipart/alternative", map[string]string{"boundary": altBoundary}), nil
		}

		if htmlBody != "" {
			return []byte(htmlBody), "text/html; charset=UTF-8", nil
		}

		return []byte(textBody), "text/plain; charset=UTF-8", nil
	}

	buf := bytes.NewBuffer(nil)
	relatedWriter := newMultipartWriter(buf)

	if hasAlternative {
		altBuf, altBoundary, err := buildAlternativePartForBodyEntity(textBody, htmlBody)
		if err != nil {
			return nil, "", err
		}

		h := textproto.MIMEHeader{}
		h.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", altBoundary))
		part, err := relatedWriter.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(altBuf.Bytes()); err != nil {
			return nil, "", err
		}
	} else {
		bodyContent := textBody
		bodyType := "text/plain"
		if htmlBody != "" {
			bodyContent = htmlBody
			bodyType = "text/html"
		}

		encoding, encodedBody, err := encodeTextBodyForMIME(bodyContent)
		if err != nil {
			return nil, "", err
		}

		h := textproto.MIMEHeader{}
		h.Set("Content-Type", bodyType+"; charset=UTF-8")
		h.Set("Content-Transfer-Encoding", encoding)
		part, err := relatedWriter.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(encodedBody); err != nil {
			return nil, "", err
		}
	}

	for _, attachment := range inlineAttachments {
		if err := writeAttachmentPart(relatedWriter, attachment); err != nil {
			return nil, "", err
		}
	}

	if err := relatedWriter.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), mediaType("multipart/related", map[string]string{"boundary": relatedWriter.Boundary()}), nil
}

func splitAttachments(attachments []Attachment) ([]Attachment, []Attachment) {
	inline := make([]Attachment, 0, len(attachments))
	regular := make([]Attachment, 0, len(attachments))

	for _, attachment := range attachments {
		if attachment.Inline {
			inline = append(inline, attachment)
			continue
		}
		regular = append(regular, attachment)
	}

	return inline, regular
}

func buildAlternativePart(textBody string, htmlBody string) (*bytes.Buffer, string, error) {
	buf := bytes.NewBuffer(nil)
	altWriter := newMultipartWriter(buf)

	plainEncoding, plainBody, err := encodeTextBodyForMIME(textBody)
	if err != nil {
		return nil, "", err
	}

	plainHeader := textproto.MIMEHeader{}
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainHeader.Set("Content-Transfer-Encoding", plainEncoding)
	plainPart, err := altWriter.CreatePart(plainHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err = plainPart.Write(plainBody); err != nil {
		return nil, "", err
	}

	htmlEncoding, htmlEncodedBody, err := encodeTextBodyForMIME(htmlBody)
	if err != nil {
		return nil, "", err
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", htmlEncoding)
	htmlPart, err := altWriter.CreatePart(htmlHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err = htmlPart.Write(htmlEncodedBody); err != nil {
		return nil, "", err
	}

	if err = altWriter.Close(); err != nil {
		return nil, "", err
	}

	return buf, altWriter.Boundary(), nil
}

func writeAttachmentPart(writer mimeMultipartWriter, attachment Attachment) error {
	contentType := attachment.ContentType
	if contentType == "" {
		contentType = detectContentType(attachment.Name)
	}

	disposition := "attachment"
	if attachment.Inline {
		disposition = "inline"
	}

	h := textproto.MIMEHeader{}
	h.Set("Content-Type", mediaType(contentType, map[string]string{"name": attachment.Name}))
	h.Set("Content-Disposition", formatContentDisposition(disposition, attachment.Name))
	h.Set("Content-Transfer-Encoding", "base64")
	if attachment.ContentID != "" {
		h.Set("Content-ID", fmt.Sprintf("<%s>", strings.Trim(attachment.ContentID, "<>")))
	}

	part, err := writer.CreatePart(h)
	if err != nil {
		return err
	}

	encoder := base64.NewEncoder(base64.StdEncoding, newBase64LineWriter(part))
	if _, err = encoder.Write(attachment.Data); err != nil {
		_ = encoder.Close()
		return err
	}

	return encoder.Close()
}
