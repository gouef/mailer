package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Message struct {
	From        []string
	To          []string
	Cc          []string
	Bcc         []string
	ReplyTo     []string
	Subject     string
	Body        string
	TextBody    string
	HTMLBody    string
	Headers     map[string][]string
	Attachments []Attachment
}

func (m *Message) Clone() *Message {
	if m == nil {
		return nil
	}

	cloned := &Message{
		From:     cloneStrings(m.From),
		To:       cloneStrings(m.To),
		Cc:       cloneStrings(m.Cc),
		Bcc:      cloneStrings(m.Bcc),
		ReplyTo:  cloneStrings(m.ReplyTo),
		Subject:  m.Subject,
		Body:     m.Body,
		TextBody: m.TextBody,
		HTMLBody: m.HTMLBody,
		Headers:  make(map[string][]string, len(m.Headers)),
	}

	for key, values := range m.Headers {
		cloned.Headers[key] = cloneStrings(values)
	}

	if len(m.Attachments) > 0 {
		cloned.Attachments = make([]Attachment, len(m.Attachments))
		for i := range m.Attachments {
			cloned.Attachments[i] = Attachment{
				Name:        m.Attachments[i].Name,
				Data:        cloneBytes(m.Attachments[i].Data),
				ContentType: m.Attachments[i].ContentType,
				Inline:      m.Attachments[i].Inline,
				ContentID:   m.Attachments[i].ContentID,
			}
		}
	}

	return cloned
}

func NewMessage(from *[]string, to *[]string, subject *string, body *string) *Message {
	m := &Message{
		Headers: make(map[string][]string),
	}

	if from != nil {
		m.From = append([]string(nil), (*from)...)
	}

	if to != nil {
		m.To = append([]string(nil), (*to)...)
	}

	if subject != nil {
		m.Subject = *subject
	}

	if body != nil {
		m.SetBody(*body)
	}

	return m
}

func New() *Message {
	return NewMessage(nil, nil, nil, nil)
}

func (m *Message) SetFrom(from string) *Message {
	m.From = []string{from}
	return m
}

func (m *Message) SetFromAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.SetFromMailbox(mailbox)
}

func (m *Message) SetFromMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.From = []string{formatted}
	return m, nil
}

func (m *Message) SetFromMailboxes(mailboxes []Address) (*Message, error) {
	if len(mailboxes) != 1 {
		return m, fmt.Errorf("exactly one from mailbox is required")
	}

	formatted, err := mailboxes[0].String()
	if err != nil {
		return m, err
	}

	m.From = []string{formatted}
	return m, nil
}

func (m *Message) AddFrom(from string) *Message {
	m.From = append(m.From, from)
	return m
}

func (m *Message) AddFromAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.AddFromMailbox(mailbox)
}

func (m *Message) AddFromMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.From = append(m.From, formatted)
	return m, nil
}

func (m *Message) SetTo(to []string) *Message {
	m.To = append([]string(nil), to...)
	return m
}

func (m *Message) SetToMailboxes(mailboxes []Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.To = formatted
	return m, nil
}

func (m *Message) AddTo(to string) *Message {
	m.To = append(m.To, to)
	return m
}

func (m *Message) AddToAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.AddToMailbox(mailbox)
}

func (m *Message) AddToMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.To = append(m.To, formatted)
	return m, nil
}

func (m *Message) AddToMailboxes(mailboxes ...Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.To = append(m.To, formatted...)
	return m, nil
}

func (m *Message) SetCc(cc []string) *Message {
	m.Cc = append([]string(nil), cc...)
	return m
}

func (m *Message) SetCcMailboxes(mailboxes []Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.Cc = formatted
	return m, nil
}

func (m *Message) AddCc(cc string) *Message {
	m.Cc = append(m.Cc, cc)
	return m
}

func (m *Message) AddCcAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.AddCcMailbox(mailbox)
}

func (m *Message) AddCcMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.Cc = append(m.Cc, formatted)
	return m, nil
}

func (m *Message) AddCcMailboxes(mailboxes ...Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.Cc = append(m.Cc, formatted...)
	return m, nil
}

func (m *Message) SetBcc(bcc []string) *Message {
	m.Bcc = append([]string(nil), bcc...)
	return m
}

func (m *Message) SetBccMailboxes(mailboxes []Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.Bcc = formatted
	return m, nil
}

func (m *Message) AddBcc(bcc string) *Message {
	m.Bcc = append(m.Bcc, bcc)
	return m
}

func (m *Message) AddBccAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.AddBccMailbox(mailbox)
}

func (m *Message) AddBccMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.Bcc = append(m.Bcc, formatted)
	return m, nil
}

func (m *Message) AddBccMailboxes(mailboxes ...Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.Bcc = append(m.Bcc, formatted...)
	return m, nil
}

func (m *Message) SetReplyTo(replyTo []string) *Message {
	m.ReplyTo = append([]string(nil), replyTo...)
	return m
}

func (m *Message) SetReplyToMailboxes(mailboxes []Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.ReplyTo = formatted
	return m, nil
}

func (m *Message) AddReplyTo(replyTo string) *Message {
	m.ReplyTo = append(m.ReplyTo, replyTo)
	return m
}

func (m *Message) AddReplyToAddress(address string, name string) (*Message, error) {
	mailbox, err := NewAddress(address, name)
	if err != nil {
		return m, err
	}

	return m.AddReplyToMailbox(mailbox)
}

func (m *Message) AddReplyToMailbox(mailbox Address) (*Message, error) {
	formatted, err := mailbox.String()
	if err != nil {
		return m, err
	}

	m.ReplyTo = append(m.ReplyTo, formatted)
	return m, nil
}

func (m *Message) AddReplyToMailboxes(mailboxes ...Address) (*Message, error) {
	formatted, err := formatMailboxList(mailboxes)
	if err != nil {
		return m, err
	}

	m.ReplyTo = append(m.ReplyTo, formatted...)
	return m, nil
}

func (m *Message) SetSubject(subject string) *Message {
	m.Subject = subject
	return m
}

func (m *Message) SetBody(body string) *Message {
	m.Body = body
	m.TextBody = body
	return m
}

func (m *Message) SetTextBody(body string) *Message {
	m.Body = body
	m.TextBody = body
	return m
}

func (m *Message) SetHtmlBody(body string) *Message {
	m.HTMLBody = body
	if m.Body == "" {
		m.Body = body
	}
	return m
}

func (m *Message) AddHeader(name string, value string) *Message {
	if !isValidHeaderName(name) {
		return m
	}

	if m.Headers == nil {
		m.Headers = make(map[string][]string)
	}

	headerName := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
	m.Headers[headerName] = append(m.Headers[headerName], value)
	return m
}

func (m *Message) SetHeader(name string, value string) *Message {
	if !isValidHeaderName(name) {
		return m
	}

	if m.Headers == nil {
		m.Headers = make(map[string][]string)
	}

	headerName := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
	m.Headers[headerName] = []string{value}
	return m
}

func (m *Message) SetListUnsubscribe(values ...string) *Message {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">") {
			parts = append(parts, trimmed)
			continue
		}
		parts = append(parts, "<"+trimmed+">")
	}

	if len(parts) == 0 {
		return m
	}

	return m.SetHeader("List-Unsubscribe", strings.Join(parts, ", "))
}

func (m *Message) SetListUnsubscribePostOneClick() *Message {
	return m.SetHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
}

func (m *Message) SetDate(date time.Time) *Message {
	return m.SetHeader("Date", date.UTC().Format(time.RFC1123Z))
}

func (m *Message) SetMessageID(messageID string) *Message {
	id := strings.TrimSpace(messageID)
	if id == "" {
		return m
	}

	if !strings.HasPrefix(id, "<") {
		id = "<" + id
	}
	if !strings.HasSuffix(id, ">") {
		id += ">"
	}

	return m.SetHeader("Message-ID", id)
}

func (m *Message) SetOrganization(organization string) *Message {
	if strings.TrimSpace(organization) == "" {
		return m
	}
	return m.SetHeader("Organization", organization)
}

func (m *Message) SetReturnPath(address string) *Message {
	parsed, err := parseAddress(address)
	if err != nil {
		return m
	}
	return m.SetHeader("Return-Path", "<"+parsed+">")
}

func (m *Message) SetReadReceiptTo(address string) *Message {
	parsed, err := parseAddress(address)
	if err != nil {
		return m
	}

	m.SetHeader("Disposition-Notification-To", parsed)
	return m.SetHeader("Return-Receipt-To", parsed)
}

func (m *Message) SetPriority(priority int) *Message {
	if priority < 1 {
		priority = 1
	}
	if priority > 5 {
		priority = 5
	}

	importance := "Normal"
	priorityText := "normal"
	if priority <= 2 {
		importance = "High"
		priorityText = "urgent"
	}
	if priority >= 4 {
		importance = "Low"
		priorityText = "non-urgent"
	}

	m.SetHeader("X-Priority", fmt.Sprintf("%d", priority))
	m.SetHeader("Importance", importance)
	return m.SetHeader("Priority", priorityText)
}

func (m *Message) GetHeader(name string) []string {
	if m.Headers == nil {
		return nil
	}

	headerName := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
	values := m.Headers[headerName]
	if values == nil {
		return nil
	}

	return append([]string(nil), values...)
}

func (m *Message) AddAttachment(name string, data []byte) *Message {
	m.Attachments = append(m.Attachments, NewAttachment(name, data))
	return m
}

func (m *Message) AddAttachmentFromPath(path string) (*Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}

	m.AddAttachment(filepath.Base(path), data)
	return m, nil
}

func (m *Message) AddEmbeddedFile(path string, contentID string) (*Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}

	m.Attachments = append(m.Attachments, NewInlineAttachment(filepath.Base(path), data, contentID))
	return m, nil
}

func (m *Message) Recipients() []string {
	recipients := make([]string, 0, len(m.To)+len(m.Cc)+len(m.Bcc))
	recipients = append(recipients, m.To...)
	recipients = append(recipients, m.Cc...)
	recipients = append(recipients, m.Bcc...)
	return recipients
}

func (m *Message) ToMIME() ([]byte, error) {
	headers, body, contentType, err := m.buildMimeBody()
	if err != nil {
		return nil, err
	}

	headers["Mime-Version"] = "1.0"
	headers["Date"] = time.Now().UTC().Format(time.RFC1123Z)
	headers["Subject"] = encodeHeaderValue("Subject", m.Subject)
	headers["Message-ID"] = m.messageID()

	if len(m.From) > 0 {
		headers["From"] = formatAddressHeaderValues(m.From)
	}

	if len(m.To) > 0 {
		headers["To"] = formatAddressHeaderValues(m.To)
	}

	if len(m.Cc) > 0 {
		headers["Cc"] = formatAddressHeaderValues(m.Cc)
	}

	if len(m.ReplyTo) > 0 {
		headers["Reply-To"] = formatAddressHeaderValues(m.ReplyTo)
	}

	customHeaders := make(map[string][]string, len(m.Headers))
	for key, values := range m.Headers {
		if !isValidHeaderName(key) {
			continue
		}

		if len(values) == 0 {
			continue
		}

		encodedValues := make([]string, 0, len(values))
		for _, value := range values {
			encodedValue := encodeHeaderValue(key, value)
			if strings.TrimSpace(encodedValue) == "" {
				continue
			}
			encodedValues = append(encodedValues, encodedValue)
		}

		if len(encodedValues) > 0 {
			customHeaders[key] = encodedValues
		}
	}

	headers["Content-Type"] = contentType

	buf := bytes.NewBuffer(nil)
	keys := make([]string, 0, len(headers)+len(customHeaders))
	for key := range headers {
		keys = append(keys, key)
	}
	for key := range customHeaders {
		if _, exists := headers[key]; exists {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if values, hasCustom := customHeaders[key]; hasCustom {
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					continue
				}
				buf.WriteString(formatHeaderLine(key, value, 78))
			}
			continue
		}

		value := headers[key]
		if strings.TrimSpace(value) != "" {
			buf.WriteString(formatHeaderLine(key, value, 78))
		}
	}
	buf.WriteString("\r\n")
	buf.Write(body)

	return buf.Bytes(), nil
}

func (m *Message) messageID() string {
	if values := m.GetHeader("Message-ID"); len(values) > 0 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}

	domain := "localhost"
	if len(m.From) > 0 {
		if parsed, err := mail.ParseAddress(strings.TrimSpace(m.From[0])); err == nil {
			if at := strings.LastIndex(parsed.Address, "@"); at >= 0 && at+1 < len(parsed.Address) {
				d := strings.TrimSpace(parsed.Address[at+1:])
				if d != "" {
					domain = d
				}
			}
		}
	}

	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("<%d@%s>", time.Now().UnixNano(), domain)
	}

	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), hex.EncodeToString(b), domain)
}

func (m *Message) envelopeFrom() (string, error) {
	if values := m.GetHeader("Return-Path"); len(values) > 0 {
		candidate := strings.TrimSpace(values[0])
		candidate = strings.Trim(candidate, "<>")
		if candidate != "" {
			return parseAddress(candidate)
		}
	}

	if len(m.From) == 0 {
		return "", fmt.Errorf("message must contain at least one sender")
	}

	return parseAddress(m.From[0])
}

func (m *Message) buildMimeBody() (map[string]string, []byte, string, error) {
	headers := make(map[string]string)

	textBody := m.TextBody
	if textBody == "" && m.HTMLBody == "" {
		textBody = m.Body
	}

	hasAlternative := textBody != "" && m.HTMLBody != ""
	inlineAttachments, regularAttachments := splitAttachments(m.Attachments)
	hasAttachments := len(inlineAttachments)+len(regularAttachments) > 0

	if !hasAlternative && !hasAttachments {
		bodyContent := textBody
		bodyType := "text/plain"
		if m.HTMLBody != "" {
			bodyContent = m.HTMLBody
			bodyType = "text/html"
		}

		encoding, encodedBody, err := encodeTextBody(bodyContent)
		if err != nil {
			return nil, nil, "", err
		}

		headers["Content-Transfer-Encoding"] = encoding
		return headers, encodedBody, bodyType + "; charset=UTF-8", nil
	}

	bodyBytes, bodyContentType, err := buildBodyEntity(textBody, m.HTMLBody, hasAlternative, inlineAttachments)
	if err != nil {
		return nil, nil, "", err
	}

	if len(regularAttachments) == 0 {
		return headers, bodyBytes, bodyContentType, nil
	}

	buf := bytes.NewBuffer(nil)
	mixedWriter := multipart.NewWriter(buf)

	bodyPartHeaders := textproto.MIMEHeader{}
	bodyPartHeaders.Set("Content-Type", bodyContentType)
	bodyPart, err := mixedWriter.CreatePart(bodyPartHeaders)
	if err != nil {
		return nil, nil, "", err
	}
	if _, err = bodyPart.Write(bodyBytes); err != nil {
		return nil, nil, "", err
	}

	for _, attachment := range regularAttachments {
		if err := writeAttachmentPart(mixedWriter, attachment); err != nil {
			return nil, nil, "", err
		}
	}

	if err := mixedWriter.Close(); err != nil {
		return nil, nil, "", err
	}

	return headers, buf.Bytes(), mediaType("multipart/mixed", map[string]string{"boundary": mixedWriter.Boundary()}), nil
}

func (m *Message) GetCc() []string {
	return m.Cc
}

func (m *Message) GetBcc() []string {
	return m.Bcc
}

func (m *Message) GetReplyTo() []string {
	return m.ReplyTo
}

func (m *Message) GetFrom() []string {
	return m.From
}

func (m *Message) GetTo() []string {
	return m.To
}

func (m *Message) GetSubject() string {
	return m.Subject
}

func (m *Message) GetBody() string {
	return m.Body
}
