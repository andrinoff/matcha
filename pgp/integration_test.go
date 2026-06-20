//go:build integration

package pgp

// Integration tests for FileBasedProvider and SmartcardProvider against real
// key material and hardware.
//
// Run with:
//
//	go test -tags integration -v ./pgp/...
//
// Required env vars (set only what the target test needs):
//
//	MATCHA_PGP_PUBLIC_KEY   path to the account public key (.asc or .gpg)
//	MATCHA_PGP_PRIVATE_KEY  path to the account private key (file-based tests)
//	MATCHA_PGP_PIN          passphrase (file) or YubiKey PIN (smartcard); may be empty
//	MATCHA_PGP_EMAIL        email address used to look up the recipient key on disk
//	MATCHA_PGP_DIR          directory holding per-address recipient public keys

import (
	"os"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set", key)
	}
	return v
}

func fileAccount(t *testing.T) (*config.Account, string) {
	t.Helper()
	pub := envOrSkip(t, "MATCHA_PGP_PUBLIC_KEY")
	priv := envOrSkip(t, "MATCHA_PGP_PRIVATE_KEY")
	pgpDir := envOrSkip(t, "MATCHA_PGP_DIR")
	return &config.Account{
		PGPPublicKey:  pub,
		PGPPrivateKey: priv,
		PGPPIN:        os.Getenv("MATCHA_PGP_PIN"),
		PGPKeySource:  "file",
	}, pgpDir
}

func yubiKeyAccount(t *testing.T) (*config.Account, string) {
	t.Helper()
	pub := envOrSkip(t, "MATCHA_PGP_PUBLIC_KEY")
	pgpDir := envOrSkip(t, "MATCHA_PGP_DIR")
	pin := envOrSkip(t, "MATCHA_PGP_PIN")

	if err := VerifyYubiKeyAvailable(); err != nil {
		t.Skipf("no YubiKey available: %v", err)
	}
	return &config.Account{
		PGPPublicKey: pub,
		PGPPIN:       pin,
		PGPKeySource: "yubikey",
	}, pgpDir
}

// ── file-based tests ─────────────────────────────────────────────────────────

// TestIntegrationFileSignVerify signs a message with the account private key
// and verifies the signature against the public keyring.
func TestIntegrationFileSignVerify(t *testing.T) {
	account, pgpDir := fileAccount(t)
	p := &FileBasedProvider{account: account, pgpDir: pgpDir}

	const body = "Integration sign+verify test.\r\n"
	payload := []byte(
		"From: test@example.com\r\n" +
			"Subject: Integration test\r\n" +
			"Content-Type: text/plain\r\n" +
			"MIME-Version: 1.0\r\n" +
			"\r\n" +
			body,
	)

	signed, err := p.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if !strings.Contains(string(signed), "multipart/signed") {
		t.Fatal("output missing multipart/signed Content-Type")
	}
	if !strings.Contains(string(signed), "BEGIN PGP") {
		t.Fatal("output missing PGP armor block")
	}

	signedPart, sigData := extractSignedPartsRaw(t, signed)
	status, err := p.Verify(signedPart, sigData)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if status != PGPStatusVerified {
		t.Errorf("Verify = %v, want PGPStatusVerified", status)
	}
	t.Logf("sign+verify OK  (signed %d bytes, signature %d bytes)", len(signedPart), len(sigData))
}

// TestIntegrationFileEncryptDecrypt encrypts a message to the recipient (whose
// key lives in MATCHA_PGP_DIR/<email>.asc) then decrypts with the account
// private key. The test exercises the sender-copy path: the sender's own
// public key is included by Encrypt so the account can read back its Sent copy.
func TestIntegrationFileEncryptDecrypt(t *testing.T) {
	account, pgpDir := fileAccount(t)
	email := envOrSkip(t, "MATCHA_PGP_EMAIL")
	p := &FileBasedProvider{account: account, pgpDir: pgpDir}

	const want = "Secret integration test payload."
	payload := []byte(
		"Content-Type: text/plain\r\n" +
			"MIME-Version: 1.0\r\n" +
			"\r\n" +
			want,
	)

	encrypted, err := p.Encrypt(payload, []string{email})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.Contains(string(encrypted), "BEGIN PGP MESSAGE") {
		t.Fatal("output missing PGP MESSAGE armor block")
	}
	t.Logf("encrypted %d → %d bytes", len(payload), len(encrypted))

	decrypted, err := p.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !strings.Contains(string(decrypted), want) {
		t.Errorf("decrypted output missing %q\ngot: %s", want, decrypted)
	}
	t.Logf("decrypt OK  (recovered %d bytes)", len(decrypted))
}

// ── YubiKey tests ─────────────────────────────────────────────────────────────

// TestIntegrationYubiKeySign signs a message using the YubiKey and verifies
// the signature against the public key on disk.
func TestIntegrationYubiKeySign(t *testing.T) {
	account, pgpDir := yubiKeyAccount(t)
	p := &SmartcardProvider{account: account, pgpDir: pgpDir}

	payload := []byte(
		"From: test@example.com\r\n" +
			"Subject: YubiKey integration test\r\n" +
			"Content-Type: text/plain\r\n" +
			"MIME-Version: 1.0\r\n" +
			"\r\n" +
			"Signed with a YubiKey.\r\n",
	)

	signed, err := p.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !strings.Contains(string(signed), "multipart/signed") {
		t.Fatal("output missing multipart/signed Content-Type")
	}
	t.Logf("signed message: %d bytes", len(signed))

	// Verify using the on-disk public key.
	fb := &FileBasedProvider{account: account, pgpDir: pgpDir}
	signedPart, sigData := extractSignedPartsRaw(t, signed)
	status, err := fb.Verify(signedPart, sigData)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if status != PGPStatusVerified {
		t.Errorf("Verify = %v, want PGPStatusVerified", status)
	}
	t.Logf("sign+verify OK")
}

// TestIntegrationYubiKeyEncryptDecrypt encrypts a message to the YubiKey owner
// (using FileBasedProvider so no hardware is needed for the encrypt step) and
// then decrypts it on-card via SmartcardProvider.
//
// This test requires an RSA decryption key on the card. Cards with ECDH /
// Curve25519 decryption keys are skipped automatically.
func TestIntegrationYubiKeyEncryptDecrypt(t *testing.T) {
	account, pgpDir := yubiKeyAccount(t)
	email := envOrSkip(t, "MATCHA_PGP_EMAIL")

	const want = "Secret YubiKey integration test payload."
	payload := []byte(
		"Content-Type: text/plain\r\n" +
			"MIME-Version: 1.0\r\n" +
			"\r\n" +
			want,
	)

	// Encrypt using only the public key — no hardware needed for this step.
	fb := &FileBasedProvider{account: account, pgpDir: pgpDir}
	encrypted, err := fb.Encrypt(payload, []string{email})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	t.Logf("encrypted %d → %d bytes", len(payload), len(encrypted))

	// Decrypt on the card.
	sc := &SmartcardProvider{account: account, pgpDir: pgpDir}
	plain, err := sc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !strings.Contains(string(plain), want) {
		t.Errorf("decrypted output missing %q\ngot: %s", want, plain)
	}
	t.Logf("decrypt OK  (recovered %d bytes)", len(plain))
}
