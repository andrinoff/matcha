package pgp

import (
	"fmt"

	cardhl "github.com/floatpane/go-openpgp-card-hl"
)

// BuildPGPSignedMessage creates a multipart/signed MIME message using a YubiKey.
// publicKeyPath is the path to the account's PGP public key file; it is used
// to read key metadata (fingerprint, algorithm, key ID) for the signature packet.
func BuildPGPSignedMessage(payload []byte, pin string, publicKeyPath string) ([]byte, error) {
	pub, err := cardhl.LoadPublicKey(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	card, err := cardhl.Open()
	if err != nil {
		return nil, err
	}
	defer card.Close() //nolint:errcheck

	return card.SignMIME(payload, pin, pub)
}

// VerifyYubiKeyAvailable checks if a YubiKey with OpenPGP support is connected.
func VerifyYubiKeyAvailable() error {
	card, err := cardhl.Open()
	if err != nil {
		return err
	}
	return card.Close()
}

// GetYubiKeyInfo returns human-readable information about the connected card.
func GetYubiKeyInfo() (string, error) {
	card, err := cardhl.Open()
	if err != nil {
		return "", err
	}
	defer card.Close() //nolint:errcheck

	info, err := card.Info()
	if err != nil {
		return "", err
	}
	return info.String(), nil
}
