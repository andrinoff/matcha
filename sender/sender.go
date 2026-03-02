package sender

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/floatpane/matcha/config"
	"go.mozilla.org/pkcs7"
)

// generateMessageID creates a unique Message-ID header.
func generateMessageID(from string) string {
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return fmt.Sprintf("<%d.%s>", time.Now().UnixNano(), from)
	}
	return fmt.Sprintf("<%x@%s>", buf, from)
}

// SendEmail constructs a multipart message with plain text, HTML, embedded images, and attachments. Includes optional S/MIME signing support.
func SendEmail(account *config.Account, to, cc, bcc []string, subject, plainBody, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME bool) error {
	smtpServer := account.GetSMTPServer()
	smtpPort := account.GetSMTPPort()

	if smtpServer == "" {
		return fmt.Errorf("unsupported or missing service_provider: %s", account.ServiceProvider)
	}

	auth := smtp.PlainAuth("", account.Email, account.Password, smtpServer)

	fromHeader := account.FetchEmail
	if account.Name != "" {
		fromHeader = fmt.Sprintf("%s <%s>", account.Name, account.FetchEmail)
	}

	// Message headers
	headers := map[string]string{
		"From":         fromHeader,
		"To":           strings.Join(to, ", "),
		"Subject":      subject,
		"Date":         time.Now().Format(time.RFC1123Z),
		"Message-ID":   generateMessageID(account.FetchEmail),
		"MIME-Version": "1.0",
	}

	if len(cc) > 0 {
		headers["Cc"] = strings.Join(cc, ", ")
	}

	if inReplyTo != "" {
		headers["In-Reply-To"] = inReplyTo
		if len(references) > 0 {
			headers["References"] = strings.Join(references, " ") + " " + inReplyTo
		} else {
			headers["References"] = inReplyTo
		}
	}

	// Build the inner message (the part that gets signed if S/MIME is enabled)
	var innerMsg bytes.Buffer
	innerWriter := multipart.NewWriter(&innerMsg)
	innerHeaders := fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())

	// --- Body Part (multipart/related) ---
	relatedHeader := textproto.MIMEHeader{}
	relatedBoundary := "related-" + innerWriter.Boundary()
	relatedHeader.Set("Content-Type", "multipart/related; boundary=\""+relatedBoundary+"\"")
	relatedPartWriter, err := innerWriter.CreatePart(relatedHeader)
	if err != nil {
		return err
	}
	relatedWriter := multipart.NewWriter(relatedPartWriter)
	relatedWriter.SetBoundary(relatedBoundary)

	// --- Alternative Part (text and html) ---
	altHeader := textproto.MIMEHeader{}
	altBoundary := "alt-" + innerWriter.Boundary()
	altHeader.Set("Content-Type", "multipart/alternative; boundary=\""+altBoundary+"\"")
	altPartWriter, err := relatedWriter.CreatePart(altHeader)
	if err != nil {
		return err
	}
	altWriter := multipart.NewWriter(altPartWriter)
	altWriter.SetBoundary(altBoundary)

	textPart, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}})
	if err != nil {
		return err
	}
	fmt.Fprint(textPart, plainBody)

	htmlPart, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}})
	if err != nil {
		return err
	}
	fmt.Fprint(htmlPart, htmlBody)
	altWriter.Close()

	// --- Inline Images ---
	for cid, data := range images {
		ext := filepath.Ext(strings.Split(cid, "@")[0])
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		imgHeader := textproto.MIMEHeader{}
		imgHeader.Set("Content-Type", mimeType)
		imgHeader.Set("Content-Transfer-Encoding", "base64")
		imgHeader.Set("Content-ID", "<"+cid+">")
		imgHeader.Set("Content-Disposition", "inline; filename=\""+cid+"\"")
		imgPart, err := relatedWriter.CreatePart(imgHeader)
		if err != nil {
			return err
		}
		imgPart.Write([]byte(wrapBase64(string(data))))
	}
	relatedWriter.Close()

	// --- Attachments ---
	for filename, data := range attachments {
		mimeType := mime.TypeByExtension(filepath.Ext(filename))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		partHeader := textproto.MIMEHeader{}
		partHeader.Set("Content-Type", mimeType)
		partHeader.Set("Content-Transfer-Encoding", "base64")
		partHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		attachmentPart, err := innerWriter.CreatePart(partHeader)
		if err != nil {
			return err
		}
		attachmentPart.Write([]byte(wrapBase64(base64.StdEncoding.EncodeToString(data))))
	}
	innerWriter.Close()

	// Compile the final payload
	var msg bytes.Buffer
	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}

	innerBodyBytes := append([]byte(innerHeaders), innerMsg.Bytes()...)

	if signSMIME {
		if account.SMIMECert == "" || account.SMIMEKey == "" {
			return errors.New("S/MIME certificate or key path is missing in account configuration")
		}

		certData, err := os.ReadFile(account.SMIMECert)
		if err != nil {
			return fmt.Errorf("failed reading smime_cert: %v", err)
		}
		keyData, err := os.ReadFile(account.SMIMEKey)
		if err != nil {
			return fmt.Errorf("failed reading smime_key: %v", err)
		}

		certBlock, _ := pem.Decode(certData)
		if certBlock == nil {
			return errors.New("failed to parse certificate PEM")
		}
		cert, err := x509.ParseCertificate(certBlock.Bytes)
		if err != nil {
			return err
		}

		keyBlock, _ := pem.Decode(keyData)
		if keyBlock == nil {
			return errors.New("failed to parse private key PEM")
		}
		privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			privKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			if err != nil {
				return err
			}
		}

		// Ensure body has canonical line endings (\r\n) for signing
		canonicalBody := bytes.ReplaceAll(innerBodyBytes, []byte("\r\n"), []byte("\n"))
		canonicalBody = bytes.ReplaceAll(canonicalBody, []byte("\n"), []byte("\r\n"))

		signedData, err := pkcs7.NewSignedData(canonicalBody)
		if err != nil {
			return err
		}
		if err := signedData.AddSigner(cert, privKey, pkcs7.SignerInfoConfig{}); err != nil {
			return err
		}
		detachedSig, err := signedData.Finish()
		if err != nil {
			return err
		}

		outerBoundary := "signed-" + innerWriter.Boundary()
		fmt.Fprintf(&msg, "Content-Type: multipart/signed; protocol=\"application/pkcs7-signature\"; micalg=\"sha-256\"; boundary=\"%s\"\r\n\r\n", outerBoundary)
		fmt.Fprintf(&msg, "This is a cryptographically signed message in MIME format.\r\n\r\n")

		fmt.Fprintf(&msg, "--%s\r\n", outerBoundary)
		msg.Write(canonicalBody)
		fmt.Fprintf(&msg, "\r\n--%s\r\n", outerBoundary)

		fmt.Fprintf(&msg, "Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n")
		fmt.Fprintf(&msg, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&msg, "Content-Disposition: attachment; filename=\"smime.p7s\"\r\n\r\n")
		msg.WriteString(wrapBase64(base64.StdEncoding.EncodeToString(detachedSig)))
		fmt.Fprintf(&msg, "\r\n--%s--\r\n", outerBoundary)

	} else {
		// Just output normal mixed multipart
		fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())
		msg.Write(innerMsg.Bytes())
	}

	allRecipients := append([]string{}, to...)
	allRecipients = append(allRecipients, cc...)
	allRecipients = append(allRecipients, bcc...)

	addr := fmt.Sprintf("%s:%d", smtpServer, smtpPort)

	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Hello("localhost"); err != nil {
		return err
	}

	if ok, _ := c.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         smtpServer,
			InsecureSkipVerify: account.Insecure,
		}
		if err = c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err = c.Auth(auth); err != nil {
				return err
			}
		}
	}

	if err = c.Mail(account.Email); err != nil {
		return err
	}
	for _, r := range allRecipients {
		if err = c.Rcpt(r); err != nil {
			return err
		}
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg.Bytes())
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	return c.Quit()
}

func wrapBase64(data string) string {
	const lineLength = 76
	var result strings.Builder
	for i := 0; i < len(data); i += lineLength {
		end := i + lineLength
		if end > len(data) {
			end = len(data)
		}
		result.WriteString(data[i:end])
		if end < len(data) {
			result.WriteString("\r\n")
		}
	}
	return result.String()
}
