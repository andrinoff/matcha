package pgp

import (
	"fmt"

	"github.com/floatpane/matcha/config"
)

// PGPStatus is the cryptographic verification state of a received message.
type PGPStatus int

const (
	PGPStatusNone       PGPStatus = iota // no PGP content
	PGPStatusVerified                    // signature present and valid
	PGPStatusUnverified                  // signature present but key missing or invalid
	PGPStatusEncrypted                   // encrypted and successfully decrypted
)

// String returns the UI badge string for a PGPStatus.
func (s PGPStatus) String() string {
	switch s {
	case PGPStatusNone:
		return ""
	case PGPStatusVerified:
		return "[PGP: Verified]"
	case PGPStatusUnverified:
		return "[PGP: Unverified]"
	case PGPStatusEncrypted:
		return "[PGP: Encrypted]"
	default:
		return ""
	}
}

// PGPProvider abstracts file-based and hardware-token PGP operations.
// All methods accept and return complete raw MIME message bytes.
type PGPProvider interface {
	// Sign wraps payload in a RFC 3156 multipart/signed MIME message.
	Sign(payload []byte) ([]byte, error)

	// Encrypt wraps payload in a RFC 3156 multipart/encrypted MIME message
	// addressed to recipients. The sender's own public key is always included
	// so the Sent copy remains readable.
	Encrypt(payload []byte, recipients []string) ([]byte, error)

	// Decrypt decrypts a multipart/encrypted MIME payload and returns the
	// inner message body.
	Decrypt(payload []byte) ([]byte, error)

	// Verify checks a detached PGP signature against signedContent (the first
	// MIME body part of a multipart/signed message). Returns PGPStatusVerified
	// when the signature is valid and a matching public key is in the keyring,
	// PGPStatusUnverified otherwise.
	Verify(signedContent, signatureData []byte) (PGPStatus, error)
}

// NewProvider returns a PGPProvider configured from account settings.
// PGPKeySource "yubikey" returns a SmartcardProvider; all other values
// (including the empty string) return a FileBasedProvider.
func NewProvider(account *config.Account) (PGPProvider, error) {
	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("pgp: cannot determine config dir: %w", err)
	}
	pgpDir := cfgDir + "/pgp"

	if account.PGPKeySource == "yubikey" {
		return &SmartcardProvider{account: account, pgpDir: pgpDir}, nil
	}
	return &FileBasedProvider{account: account, pgpDir: pgpDir}, nil
}
