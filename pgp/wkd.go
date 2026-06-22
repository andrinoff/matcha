package pgp

import (
	"bytes"
	"crypto/sha1"
	"encoding/base32"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/floatpane/matcha/internal/httpclient"
)

// wkdDirectURL builds the "direct" WKD URL for an email address.
// See https://datatracker.ietf.org/doc/draft-koch-openpgp-webkey-service/
//
// The direct method uses:
//
//	https://<domain>/.well-known/openpgpkey/hu/<z-base-32-hash>?l=<local-part>
func wkdDirectURL(email string) (string, error) {
	local, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || local == "" || domain == "" {
		return "", fmt.Errorf("wkd: invalid email %q", email)
	}

	h := sha1.Sum([]byte(local))
	hash := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h[:]))

	return fmt.Sprintf("https://%s/.well-known/openpgpkey/hu/%s?l=%s", domain, hash, local), nil
}

// LookupWKD attempts to fetch a PGP public key via the Web Key Directory
// protocol for the given email address. Returns the parsed OpenPGP entity on
// success. On any failure (network, parsing, no key published) it returns an
// error and does NOT cache anything to disk.
func LookupWKD(email string) (*openpgp.Entity, error) {
	url, err := wkdDirectURL(email)
	if err != nil {
		return nil, err
	}

	client := httpclient.New(httpclient.WKDLookupTimeout)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("wkd: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wkd: %s returned status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MiB
	if err != nil {
		return nil, fmt.Errorf("wkd: read response: %w", err)
	}

	entityList, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(body))
	if err != nil {
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("wkd: parse key: %w", err)
		}
	}
	if len(entityList) == 0 {
		return nil, fmt.Errorf("wkd: no keys in response from %s", url)
	}
	return entityList[0], nil
}

// CacheWKDKey saves an armored public key to the pgp directory so subsequent
// lookups hit disk instead of the network. The file is stored as <email>.asc.
func CacheWKDKey(pgpDir, email string, entity *openpgp.Entity) error {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return fmt.Errorf("wkd: armor encode: %w", err)
	}
	if err := entity.Serialize(w); err != nil {
		return fmt.Errorf("wkd: serialize: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("wkd: armor close: %w", err)
	}

	if err := os.MkdirAll(pgpDir, 0700); err != nil {
		return fmt.Errorf("wkd: mkdir: %w", err)
	}

	path := filepath.Join(pgpDir, strings.ToLower(strings.TrimSpace(email))+".asc")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("wkd: write cache: %w", err)
	}
	return nil
}

// MissingLocalKeys returns the subset of recipient emails that don't have a
// public key file in pgpDir. Used by the composer to decide whether to prompt
// for WKD download before enabling encryption.
func MissingLocalKeys(pgpDir string, recipients []string) []string {
	var missing []string
	for _, r := range recipients {
		email := strings.ToLower(strings.TrimSpace(r))
		if i := strings.Index(email, "<"); i >= 0 {
			email = strings.TrimSuffix(email[i+1:], ">")
		}
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		found := false
		for _, ext := range []string{".asc", ".gpg", ".pem"} {
			if _, err := os.Stat(filepath.Join(pgpDir, email+ext)); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, email)
		}
	}
	return missing
}
