package pgp

import (
	"errors"
	"fmt"

	cardhl "github.com/floatpane/go-openpgp-card-hl"

	"github.com/floatpane/matcha/config"
)

// SmartcardProvider implements PGPProvider using a hardware OpenPGP token
// (e.g. YubiKey). The device's private key never leaves the hardware.
type SmartcardProvider struct {
	account *config.Account
	pgpDir  string
}

// Sign delegates to BuildPGPSignedMessage, which manages the PC/SC session
// and produces a RFC 3156 multipart/signed message.
func (p *SmartcardProvider) Sign(payload []byte) ([]byte, error) {
	if p.account.PGPPIN == "" {
		return nil, errors.New("pgp smartcard: PIN not configured")
	}
	if p.account.PGPPublicKey == "" {
		return nil, errors.New("pgp smartcard: public key path not configured")
	}
	return BuildPGPSignedMessage(payload, p.account.PGPPIN, p.account.PGPPublicKey)
}

// Encrypt uses recipient public keys from disk; no private key is required,
// so this delegates directly to FileBasedProvider.
func (p *SmartcardProvider) Encrypt(payload []byte, recipients []string) ([]byte, error) {
	return (&FileBasedProvider{account: p.account, pgpDir: p.pgpDir}).Encrypt(payload, recipients)
}

// Decrypt decrypts a multipart/encrypted message using the card's on-device
// decryption key. The private key never leaves the hardware.
//
// Only RSA decryption keys are supported by the PC/SC interface; for
// ECDH/Curve25519 keys use a gpg-agent backed flow instead.
func (p *SmartcardProvider) Decrypt(payload []byte) ([]byte, error) {
	if p.account.PGPPIN == "" {
		return nil, errors.New("pgp smartcard: PIN not configured")
	}
	if p.account.PGPPublicKey == "" {
		return nil, errors.New("pgp smartcard: public key path not configured")
	}

	pubEntity, err := cardhl.LoadEntity(p.account.PGPPublicKey)
	if err != nil {
		return nil, fmt.Errorf("pgp smartcard: load public key: %w", err)
	}

	card, err := cardhl.Open()
	if err != nil {
		return nil, err
	}
	defer card.Close() //nolint:errcheck

	plain, err := card.DecryptMIME(payload, p.account.PGPPIN, pubEntity)
	if err == nil {
		return plain, nil
	}
	if errors.Is(err, cardhl.ErrUnsupportedKey) {
		return gpgAgentDecryptMIME(payload, p.account.PGPPIN)
	}
	return nil, err
}

// Verify delegates to FileBasedProvider since signature verification requires
// only public keys, which are always available on disk.
func (p *SmartcardProvider) Verify(signedContent, signatureData []byte) (PGPStatus, error) {
	return (&FileBasedProvider{account: p.account, pgpDir: p.pgpDir}).Verify(signedContent, signatureData)
}
