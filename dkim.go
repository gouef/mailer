package mailer

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

type DKIMSigner struct {
	Domain     string
	Selector   string
	Identity   string
	Headers    []string
	PrivateKey *rsa.PrivateKey
}

func NewDKIMSigner(domain string, selector string, privateKeyPEM []byte) (*DKIMSigner, error) {
	key, err := parseDKIMPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	return &DKIMSigner{
		Domain:     strings.TrimSpace(domain),
		Selector:   strings.TrimSpace(selector),
		PrivateKey: key,
	}, nil
}

func (s *DKIMSigner) Sign(mime []byte) ([]byte, error) {
	if s == nil {
		return nil, errors.New("dkim signer is nil")
	}

	if strings.TrimSpace(s.Domain) == "" {
		return nil, errors.New("dkim domain is required")
	}

	if strings.TrimSpace(s.Selector) == "" {
		return nil, errors.New("dkim selector is required")
	}

	if s.PrivateKey == nil {
		return nil, errors.New("dkim private key is required")
	}

	headerRaw, bodyRaw, err := splitMIMEMessage(mime)
	if err != nil {
		return nil, err
	}

	headers := parseHeaderFields(headerRaw)
	h := s.resolveSignedHeaders(headers)
	if len(h) == 0 {
		return nil, errors.New("dkim: no headers to sign")
	}

	bodyCanonical := relaxedBodyCanonicalization(bodyRaw)
	bodyHash := sha256.Sum256([]byte(bodyCanonical))
	bh := base64.StdEncoding.EncodeToString(bodyHash[:])

	hList := strings.Join(h, ":")
	timestamp := time.Now().Unix()

	dkimValues := []string{
		"v=1",
		"a=rsa-sha256",
		"c=relaxed/relaxed",
		"d=" + s.Domain,
		"s=" + s.Selector,
		"t=" + fmt.Sprintf("%d", timestamp),
		"h=" + hList,
		"bh=" + bh,
	}
	if strings.TrimSpace(s.Identity) != "" {
		dkimValues = append(dkimValues, "i="+s.Identity)
	}

	dkimUnsigned := strings.Join(dkimValues, "; ") + "; b="

	canonical := bytes.NewBuffer(nil)
	for _, name := range h {
		value, ok := lastHeaderValue(headers, name)
		if !ok {
			continue
		}
		canonical.WriteString(canonicalizeHeaderRelaxed(name, value))
	}
	canonical.WriteString(canonicalizeHeaderRelaxed("DKIM-Signature", dkimUnsigned))

	hash := sha256.Sum256(canonical.Bytes())
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.PrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return nil, fmt.Errorf("dkim sign failed: %w", err)
	}

	encodedSig := base64.StdEncoding.EncodeToString(signature)
	dkimHeader := "DKIM-Signature: " + dkimUnsigned + encodedSig + "\r\n"

	final := bytes.NewBuffer(nil)
	final.WriteString(dkimHeader)
	final.Write(mime)

	return final.Bytes(), nil
}

func parseDKIMPrivateKey(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("dkim private key pem is invalid")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	pkcs8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("dkim private key parse failed: %w", err)
	}

	rsaKey, ok := pkcs8.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("dkim private key is not rsa")
	}

	return rsaKey, nil
}

func splitMIMEMessage(mime []byte) (string, string, error) {
	text := string(mime)
	parts := strings.SplitN(text, "\r\n\r\n", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid mime message")
	}

	return parts[0], parts[1], nil
}

func parseHeaderFields(headerRaw string) map[string][]string {
	result := make(map[string][]string)
	lines := strings.Split(headerRaw, "\r\n")

	var currentName string
	var currentValue strings.Builder

	flush := func() {
		if currentName == "" {
			return
		}
		name := strings.ToLower(strings.TrimSpace(currentName))
		result[name] = append(result[name], currentValue.String())
		currentName = ""
		currentValue.Reset()
	}

	for _, line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if currentName != "" {
				currentValue.WriteByte(' ')
				currentValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		flush()

		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}

		currentName = line[:idx]
		currentValue.WriteString(strings.TrimSpace(line[idx+1:]))
	}

	flush()
	return result
}

func (s *DKIMSigner) resolveSignedHeaders(headers map[string][]string) []string {
	if len(s.Headers) > 0 {
		values := make([]string, 0, len(s.Headers))
		for _, name := range s.Headers {
			lower := strings.ToLower(strings.TrimSpace(name))
			if lower == "" {
				continue
			}
			if _, ok := headers[lower]; ok {
				values = append(values, lower)
			}
		}
		return values
	}

	defaults := []string{"from", "to", "subject", "date", "message-id", "mime-version", "content-type"}
	values := make([]string, 0, len(defaults))
	for _, name := range defaults {
		if _, ok := headers[name]; ok {
			values = append(values, name)
		}
	}

	return values
}

func lastHeaderValue(headers map[string][]string, name string) (string, bool) {
	values, ok := headers[strings.ToLower(strings.TrimSpace(name))]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

func canonicalizeHeaderRelaxed(name string, value string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	normalized := strings.Join(strings.Fields(value), " ")
	return lower + ":" + normalized + "\r\n"
}

func relaxedBodyCanonicalization(body string) string {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	for i := range lines {
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) == 0 {
		return "\r\n"
	}

	return strings.Join(lines, "\r\n") + "\r\n"
}
