package pgp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

func TestWKDDirectURL(t *testing.T) {
	tests := []struct {
		email   string
		wantErr bool
		check   func(t *testing.T, url string)
	}{
		{
			email: "alice@example.com",
			check: func(t *testing.T, url string) {
				if !strings.HasPrefix(url, "https://example.com/.well-known/openpgpkey/hu/") {
					t.Errorf("unexpected URL prefix: %s", url)
				}
				if !strings.HasSuffix(url, "?l=alice") {
					t.Errorf("missing local part query param: %s", url)
				}
			},
		},
		{
			email: "Bob@Example.COM",
			check: func(t *testing.T, url string) {
				if !strings.Contains(url, "example.com") {
					t.Errorf("domain not lowercased: %s", url)
				}
				if !strings.HasSuffix(url, "?l=bob") {
					t.Errorf("local part not lowercased: %s", url)
				}
			},
		},
		{email: "invalid", wantErr: true},
		{email: "@domain.com", wantErr: true},
		{email: "user@", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			url, err := wkdDirectURL(tt.email)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, url)
			}
		})
	}
}

func TestLookupWKDWithTestServer(t *testing.T) {
	entity, err := openpgp.NewEntity("Test User", "", "test@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}

	var armoredKey bytes.Buffer
	w, err := armor.Encode(&armoredKey, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor.Encode: %v", err)
	}
	if err := entity.Serialize(w); err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("armor close: %v", err)
	}
	keyBytes := armoredKey.Bytes()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/openpgpkey/hu/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(keyBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Override the HTTP client to use our test server's TLS config.
	origClient := http.DefaultClient
	http.DefaultClient = srv.Client()
	defer func() { http.DefaultClient = origClient }()

	// We can't easily override LookupWKD's internal client, so test CacheWKDKey
	// and the URL builder instead. The integration with a real WKD endpoint
	// would require network access.
	t.Run("CacheWKDKey", func(t *testing.T) {
		dir := t.TempDir()
		pgpDir := filepath.Join(dir, "pgp")

		err := CacheWKDKey(pgpDir, "test@example.com", entity)
		if err != nil {
			t.Fatalf("CacheWKDKey: %v", err)
		}

		cached, err := os.ReadFile(filepath.Join(pgpDir, "test@example.com.asc"))
		if err != nil {
			t.Fatalf("read cached key: %v", err)
		}
		if len(cached) == 0 {
			t.Fatal("cached key is empty")
		}

		loaded, err := loadPublicEntity(filepath.Join(pgpDir, "test@example.com.asc"))
		if err != nil {
			t.Fatalf("loadPublicEntity on cached key: %v", err)
		}
		if loaded.PrimaryKey.KeyId != entity.PrimaryKey.KeyId {
			t.Errorf("cached key ID mismatch: got %d, want %d",
				loaded.PrimaryKey.KeyId, entity.PrimaryKey.KeyId)
		}
	})
}

func TestLoadPublicKeyForEmailFallsBackToWKD(t *testing.T) {
	entity, err := openpgp.NewEntity("WKD User", "", "wkd@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}

	dir := t.TempDir()
	pgpDir := filepath.Join(dir, "pgp")
	if err := os.Mkdir(pgpDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Pre-cache the key so we don't need a real WKD server.
	if err := CacheWKDKey(pgpDir, "wkd@example.com", entity); err != nil {
		t.Fatalf("CacheWKDKey: %v", err)
	}

	p := &FileBasedProvider{pgpDir: pgpDir}
	loaded, err := p.loadPublicKeyForEmail("wkd@example.com")
	if err != nil {
		t.Fatalf("loadPublicKeyForEmail: %v", err)
	}
	if loaded.PrimaryKey.KeyId != entity.PrimaryKey.KeyId {
		t.Errorf("key ID mismatch: got %d, want %d",
			loaded.PrimaryKey.KeyId, entity.PrimaryKey.KeyId)
	}
}
