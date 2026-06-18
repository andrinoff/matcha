package pgp

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"os/exec"
	"strings"
)

// gpgAgentDecryptMIME decrypts a multipart/encrypted MIME message (RFC 3156)
// by piping the OpenPGP ciphertext to the local gpg binary, which delegates
// to gpg-agent. Supports all key types including ECDH/Curve25519 YubiKey slots.
//
// pin is the YubiKey User PIN (or software key passphrase). It is forwarded to
// gpg-agent via --pinentry-mode loopback so no Pinentry GUI is required.
func gpgAgentDecryptMIME(payload []byte, pin string) ([]byte, error) {
	armored, err := extractArmoredFromMIME(payload)
	if err != nil {
		return nil, fmt.Errorf("gpg-agent: %w", err)
	}

	// Pipe the PIN to gpg on fd 3 so stdin stays free for the ciphertext.
	pinR, pinW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("gpg-agent: pin pipe: %w", err)
	}
	if _, err := io.WriteString(pinW, pin+"\n"); err != nil {
		pinR.Close()
		pinW.Close()
		return nil, fmt.Errorf("gpg-agent: write pin: %w", err)
	}
	pinW.Close()

	cmd := exec.Command("gpg",
		"--decrypt",
		"--batch",
		"--yes",
		"--quiet",
		"--pinentry-mode", "loopback",
		"--passphrase-fd", "3",
	)
	cmd.Stdin = bytes.NewReader(armored)
	cmd.ExtraFiles = []*os.File{pinR} // becomes fd 3
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	pinR.Close()
	if runErr != nil {
		return nil, fmt.Errorf("gpg-agent decrypt: %w: %s", runErr, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// extractArmoredFromMIME returns the ASCII-armored OpenPGP ciphertext from
// the data part of a multipart/encrypted MIME message. Unlike the cardhl
// library's internal helper, this preserves the armor rather than decoding it,
// so the bytes can be piped directly to gpg.
func extractArmoredFromMIME(payload []byte) ([]byte, error) {
	sep := bytes.Index(payload, []byte("\r\n\r\n"))
	bodyOffset := 4
	if sep < 0 {
		sep = bytes.Index(payload, []byte("\n\n"))
		bodyOffset = 2
	}
	if sep < 0 {
		return nil, fmt.Errorf("no header/body separator")
	}

	unfolded := strings.ReplaceAll(string(payload[:sep]), "\r\n\t", " ")
	unfolded = strings.ReplaceAll(unfolded, "\r\n ", " ")
	unfolded = strings.ReplaceAll(unfolded, "\n\t", " ")
	unfolded = strings.ReplaceAll(unfolded, "\n ", " ")

	var contentType string
	for _, line := range strings.Split(unfolded, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.ToUpper(line), "CONTENT-TYPE:") {
			contentType = strings.TrimSpace(line[len("Content-Type:"):])
			break
		}
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/encrypted") {
		return nil, fmt.Errorf("expected multipart/encrypted, got %q", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("missing boundary parameter")
	}

	mr := multipart.NewReader(bytes.NewReader(payload[sep+bodyOffset:]), boundary)

	// Discard the RFC 3156 control part ("Version: 1").
	if _, err := mr.NextPart(); err != nil {
		return nil, fmt.Errorf("missing control part: %w", err)
	}

	dataPart, err := mr.NextPart()
	if err != nil {
		return nil, fmt.Errorf("missing data part: %w", err)
	}
	defer dataPart.Close() //nolint:errcheck

	return io.ReadAll(dataPart)
}
