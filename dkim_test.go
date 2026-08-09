package mailer

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestDKIMSignerAddsHeader(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("DKIM test").
		SetTextBody("body")

	raw, err := message.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	signed, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	text := string(signed)
	if !strings.HasPrefix(text, "DKIM-Signature: ") {
		t.Fatalf("missing dkim signature header")
	}

	if !strings.Contains(text, " d=example.com;") || !strings.Contains(text, " s=mail;") {
		t.Fatalf("missing dkim domain/selector attributes")
	}

	if !strings.Contains(text, " bh=") || !strings.Contains(text, " b=") {
		t.Fatalf("missing dkim hash/signature attributes")
	}
}

func TestSMTPMailerInterceptorAndDKIM(t *testing.T) {
	server := startSMTPTestServer(t, false)
	defer server.close()

	host, port := server.hostPort(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("SMTP DKIM test").
		SetTextBody("hello")

	mailer := NewSMTPMailer(host, port).
		SetSecurity(SecurityNone).
		SetTimeout(2 * time.Second).
		Use(func(m *Message) error {
			m.SetHeader("X-Intercepted", "yes")
			return nil
		}).
		SetDKIMSigner(signer)

	if err := mailer.Send(message); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	body := server.message()
	if !strings.Contains(body, "X-Intercepted: yes") {
		t.Fatalf("expected interceptor header in smtp payload")
	}

	if !strings.Contains(body, "DKIM-Signature: ") {
		t.Fatalf("expected dkim header in smtp payload")
	}
}

func TestDKIMSignerCryptographicVerification(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("DKIM verify").
		SetTextBody("line 1\nline 2\n")

	raw, err := message.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	signed, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if err := verifySignedMessage(privateKey, signed); err != nil {
		t.Fatalf("verification failed: %v", err)
	}
}

func TestDKIMSignerVerificationFailsWhenBodyChanges(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("DKIM tamper body").
		SetTextBody("original body")

	raw, err := message.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	signed, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	tampered := []byte(strings.Replace(string(signed), "original body", "tampered body", 1))
	if err := verifySignedMessage(privateKey, tampered); err == nil {
		t.Fatalf("expected verification failure after body tampering")
	}
}

func TestDKIMSignerVerificationFailsWhenSignedHeaderChanges(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner failed: %v", err)
	}

	message := New().
		SetFrom("sender@example.com").
		AddTo("recipient@example.com").
		SetSubject("DKIM original subject").
		SetTextBody("body")

	raw, err := message.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	signed, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	tampered := []byte(strings.Replace(string(signed), "Subject: DKIM original subject", "Subject: DKIM changed subject", 1))
	if err := verifySignedMessage(privateKey, tampered); err == nil {
		t.Fatalf("expected verification failure after signed header tampering")
	}
}

func verifySignedMessage(privateKey *rsa.PrivateKey, signed []byte) error {
	if privateKey == nil {
		return errors.New("private key is nil")
	}

	headerRaw, bodyRaw, err := splitMIMEMessage(signed)
	if err != nil {
		return err
	}

	headers := parseHeaderFields(headerRaw)
	dkimHeaderValue, ok := lastHeaderValue(headers, "dkim-signature")
	if !ok {
		return errors.New("DKIM-Signature header missing")
	}

	tags := parseDKIMTags(dkimHeaderValue)
	if tags["bh"] == "" || tags["b"] == "" {
		return errors.New("DKIM bh or b tag missing")
	}

	signedHeaders := strings.Split(tags["h"], ":")
	if len(signedHeaders) == 0 {
		return errors.New("signed header list is empty")
	}

	bodyCanonical := relaxedBodyCanonicalization(bodyRaw)
	bodyHash := sha256.Sum256([]byte(bodyCanonical))
	expectedBH := base64.StdEncoding.EncodeToString(bodyHash[:])
	if tags["bh"] != expectedBH {
		return errors.New("body hash mismatch")
	}

	canonical := strings.Builder{}
	for _, name := range signedHeaders {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" {
			continue
		}
		value, exists := lastHeaderValue(headers, name)
		if !exists {
			return errors.New("signed header is missing")
		}
		canonical.WriteString(canonicalizeHeaderRelaxed(name, value))
	}

	unsignedValue, err := stripDKIMSignatureB(dkimHeaderValue)
	if err != nil {
		return err
	}
	canonical.WriteString(canonicalizeHeaderRelaxed("DKIM-Signature", unsignedValue))

	signatureBytes, err := base64.StdEncoding.DecodeString(tags["b"])
	if err != nil {
		return err
	}

	hashed := sha256.Sum256([]byte(canonical.String()))
	if err := rsa.VerifyPKCS1v15(&privateKey.PublicKey, crypto.SHA256, hashed[:], signatureBytes); err != nil {
		return err
	}

	return nil
}

func parseDKIMTags(value string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(value, ";")
	for _, part := range parts {
		piece := strings.TrimSpace(part)
		if piece == "" {
			continue
		}
		idx := strings.IndexByte(piece, '=')
		if idx <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(piece[:idx]))
		v := strings.TrimSpace(piece[idx+1:])
		result[k] = v
	}
	return result
}

func stripDKIMSignatureB(value string) (string, error) {
	re := regexp.MustCompile(`(?i)\bb\s*=\s*[^;]*`)
	stripped := re.ReplaceAllString(value, "b=")
	if strings.TrimSpace(stripped) == value {
		return "", errors.New("dkim signature is missing b= tag")
	}
	return stripped, nil
}

func TestDKIMValidationAndHelperCoverage(t *testing.T) {
	if _, err := NewDKIMSigner("example.com", "mail", []byte("bad pem")); err == nil {
		t.Fatalf("expected invalid pem error")
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key failed: %v", err)
	}
	_ = pub
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8 failed: %v", err)
	}
	nonRSAKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if _, err := parseDKIMPrivateKey(nonRSAKey); err == nil {
		t.Fatalf("expected non-rsa key error")
	}

	garbagePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage-key-bytes")})
	if _, err := parseDKIMPrivateKey(garbagePEM); err == nil {
		t.Fatalf("expected parse failure for invalid key bytes in valid pem block")
	}

	if _, _, err := splitMIMEMessage([]byte("no-separator")); err == nil {
		t.Fatalf("expected invalid mime split error")
	}

	headers := parseHeaderFields("Subject: one\r\n folded\r\nX-NoColon\r\nTo: two\r\n")
	if got := headers["subject"][0]; got != "one folded" {
		t.Fatalf("unexpected folded header parse: %q", got)
	}

	signer := &DKIMSigner{Headers: []string{" Subject ", "", "Missing"}}
	resolved := signer.resolveSignedHeaders(map[string][]string{"subject": {"x"}})
	if len(resolved) != 1 || resolved[0] != "subject" {
		t.Fatalf("unexpected resolved headers: %v", resolved)
	}

	defaults := (&DKIMSigner{}).resolveSignedHeaders(map[string][]string{"from": {"a"}, "date": {"b"}})
	if len(defaults) != 2 {
		t.Fatalf("unexpected default headers: %v", defaults)
	}

	if _, ok := lastHeaderValue(map[string][]string{"x": {"1", "2"}}, "x"); !ok {
		t.Fatalf("expected existing header value")
	}
	if _, ok := lastHeaderValue(map[string][]string{"x": {}}, "x"); ok {
		t.Fatalf("expected missing last header value")
	}

	if got := relaxedBodyCanonicalization("line 1  \r\n\r\n"); got != "line 1\r\n" {
		t.Fatalf("unexpected body canonicalization: %q", got)
	}
}

func TestDKIMSignValidationErrors(t *testing.T) {
	if _, err := (*DKIMSigner)(nil).Sign([]byte("x")); err == nil {
		t.Fatalf("expected nil signer error")
	}

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	signer := &DKIMSigner{Selector: "sel", PrivateKey: key}
	if _, err := signer.Sign([]byte("Header: x\r\n\r\nbody")); err == nil {
		t.Fatalf("expected missing domain error")
	}

	signer = &DKIMSigner{Domain: "example.com", PrivateKey: key}
	if _, err := signer.Sign([]byte("Header: x\r\n\r\nbody")); err == nil {
		t.Fatalf("expected missing selector error")
	}

	signer = &DKIMSigner{Domain: "example.com", Selector: "sel"}
	if _, err := signer.Sign([]byte("Header: x\r\n\r\nbody")); err == nil {
		t.Fatalf("expected missing private key error")
	}

	signer = &DKIMSigner{Domain: "example.com", Selector: "sel", PrivateKey: key}
	if _, err := signer.Sign([]byte("bad")); err == nil {
		t.Fatalf("expected invalid mime message error")
	}

	if _, err := signer.Sign([]byte("X-Test: 1\r\n\r\nbody")); err == nil {
		t.Fatalf("expected no headers to sign error")
	}
}

func TestDKIMPKCS8RSAAndIdentityBranch(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal pkcs8 failed: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	signer, err := NewDKIMSigner("example.com", "mail", pemKey)
	if err != nil {
		t.Fatalf("NewDKIMSigner from PKCS8 failed: %v", err)
	}
	signer.Identity = "sender@example.com"
	signer.Headers = []string{"from", "subject", "x-missing"}

	message := New().SetFrom("sender@example.com").AddTo("recipient@example.com").SetSubject("id").SetTextBody("body")
	raw, err := message.ToMIME()
	if err != nil {
		t.Fatalf("ToMIME failed: %v", err)
	}

	signed, err := signer.Sign(raw)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	text := string(signed)
	if !strings.Contains(text, " i=sender@example.com;") {
		t.Fatalf("expected identity tag in dkim signature, got:\n%s", text)
	}
	if !strings.Contains(text, " h=from:subject;") {
		t.Fatalf("expected resolved custom header list in dkim signature, got:\n%s", text)
	}
}
