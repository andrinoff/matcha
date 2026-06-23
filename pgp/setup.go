package pgp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/floatpane/matcha/config"
)

// HasSMIMESetup reports whether the account has S/MIME signing configured.
func HasSMIMESetup(acc *config.Account) bool {
	return acc != nil && acc.SMIMECert != "" && acc.SMIMEKey != ""
}

// HasSMIMECertForRecipient reports whether a usable S/MIME certificate exists
// for the given recipient. For the account's own address it checks the
// configured S/MIME certificate; otherwise it looks in the certs directory.
func HasSMIMECertForRecipient(recipient string, acc *config.Account) bool {
	if acc == nil {
		return false
	}
	email := strings.ToLower(strings.TrimSpace(recipient))
	if email == "" {
		return false
	}
	if strings.EqualFold(email, acc.Email) && acc.SMIMECert != "" {
		return true
	}
	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return false
	}
	certPath := filepath.Join(cfgDir, "certs", email+".pem")
	if _, err := os.Stat(certPath); err == nil {
		return true
	}
	return false
}

// HasPGPSetup reports whether the account has PGP configured (file-based or
// YubiKey hardware key).
func HasPGPSetup(acc *config.Account) bool {
	return acc != nil && (acc.PGPKeySource != "" || acc.PGPPublicKey != "")
}

// HasLocalKeyForRecipient reports whether a public key file for the recipient
// exists in the configured PGP directory.
func HasLocalKeyForRecipient(recipient string) (bool, error) {
	email := strings.ToLower(strings.TrimSpace(recipient))
	if email == "" {
		return false, nil
	}
	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return false, err
	}
	pgpDir := filepath.Join(cfgDir, "pgp")
	for _, ext := range []string{".asc", ".gpg", ".pem"} {
		if _, err := os.Stat(filepath.Join(pgpDir, email+ext)); err == nil {
			return true, nil
		}
	}
	return false, nil
}
