package signing

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
	"howett.net/plist"
	modernpkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
	signingpkg "github.com/rudrankriyam/App-Store-Connect-CLI/internal/signing"
)

const (
	importSourcePassword      = "source-secret-7f3a"
	importDestinationPassword = "destination-password"
	importProfileUUID         = "6f1c2a3b-4d5e-4f60-8a7b-9c0d1e2f3a4b"
)

// encryptOpenSSLV1 is a reference v1 encoder for importSourcePassword: base64("Salted__" || salt ||
// AES-256-CBC(PKCS#7)) with an MD5 EVP_BytesToKey key and IV.
func encryptOpenSSLV1(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	var material, previous []byte
	for len(material) < 48 {
		sum := md5.Sum(append(append(append([]byte{}, previous...), importSourcePassword...), salt...))
		previous = sum[:]
		material = append(material, previous...)
	}
	block, err := aes.NewCipher(material[:32])
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, material[32:48]).CryptBlocks(ciphertext, padded)
	return []byte(base64.StdEncoding.EncodeToString(append(append([]byte("Salted__"), salt...), ciphertext...)))
}

// encryptOpenSSLV2 is a reference v2 encoder for importSourcePassword: base64(marker (20) || salt ||
// tag || AES-256-GCM) with key, nonce, and additional data from PBKDF2.
func encryptOpenSSLV2(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	material := pbkdf2.Key([]byte(importSourcePassword), salt, 10_000, 68, sha256.New)
	block, err := aes.NewCipher(material[:32])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, material[32:44], plaintext, material[44:])
	ciphertext, tag := sealed[:len(sealed)-16], sealed[len(sealed)-16:]
	encoded := []byte(strings.Repeat("v", 20))
	encoded = append(append(append(encoded, salt...), tag...), ciphertext...)
	return []byte(base64.StdEncoding.EncodeToString(encoded) + "\n")
}

type openSSLImportFixture struct {
	dir             string
	distribution    *x509.Certificate
	development     *x509.Certificate
	distributionSHA string
	developmentSHA  string
}

func newOpenSSLImportFixture(t *testing.T) openSSLImportFixture {
	t.Helper()
	dir := t.TempDir()
	write := func(relPath string, data []byte) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	distributionKey := mustRSAKey(t)
	distribution := mustSigningCertificate(t, distributionKey, 101)
	keyDER, err := x509.MarshalPKCS8PrivateKey(distributionKey)
	if err != nil {
		t.Fatal(err)
	}
	write("certs/distribution/CERT1.cer", encryptOpenSSLV1(t, distribution.Raw))
	write("certs/distribution/CERT1.p12", encryptOpenSSLV1(t, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))

	developmentKey := mustECKey(t)
	development := mustSigningCertificate(t, developmentKey, 202)
	identity, err := modernpkcs12.Legacy.Encode(developmentKey, development, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	write("certs/development/CERT2.cer", encryptOpenSSLV2(t, development.Raw))
	write("certs/development/CERT2.p12", encryptOpenSSLV2(t, identity))

	payload, err := plist.Marshal(map[string]any{
		"Name":                        "App Store com.example.app",
		"UUID":                        importProfileUUID,
		"TeamIdentifier":              []string{"TEAM123"},
		"ApplicationIdentifierPrefix": []string{"TEAM123"},
		"Platform":                    []string{"iOS"},
		"ExpirationDate":              time.Now().Add(12 * time.Hour).UTC().Truncate(time.Second),
		"DeveloperCertificates":       [][]byte{distribution.Raw},
		"Entitlements": map[string]any{
			"application-identifier": "TEAM123.com.example.app",
			"get-task-allow":         false,
		},
	}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	write("profiles/appstore/AppStore_com.example.app.mobileprovision",
		encryptOpenSSLV2(t, mustSignedCMS(t, payload, distribution, distributionKey)))
	write("README.md", []byte("unencrypted"))

	return openSSLImportFixture{
		dir:             dir,
		distribution:    distribution,
		development:     development,
		distributionSHA: certificateSHA256(distribution),
		developmentSHA:  certificateSHA256(development),
	}
}

func runSigningSyncImport(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := syncImportCommand()
	cmd.FlagSet.SetOutput(io.Discard)
	var runErr error
	stdout, stderr := captureOutput(t, func() {
		if err := cmd.Parse(args); err != nil {
			t.Fatalf("parse: %v", err)
		}
		runErr = cmd.Run(context.Background())
	})
	if strings.Contains(stdout+stderr, importSourcePassword) {
		t.Fatalf("source password leaked to output:\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	return stdout, stderr, runErr
}

func TestSigningSyncImportWritesPullableStore(t *testing.T) {
	fixture := newOpenSSLImportFixture(t)
	remoteURL, remotePath := newSigningSyncBareRemote(t)
	passwordFile := filepath.Join(t.TempDir(), "password")
	writePrivateTestFile(t, passwordFile, []byte(importDestinationPassword))
	args := []string{
		"--source", fixture.dir, "--source-password", importSourcePassword,
		"--repo", remoteURL, "--password-file", passwordFile, "--confirm", "--output", "json",
	}

	stdout, stderr, err := runSigningSyncImport(t, args...)
	if err != nil {
		t.Fatalf("import: %v\nstderr=%s", err, stderr)
	}
	var result asc.SigningSyncImportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout)
	}
	if result.PublicationState != "succeeded" || len(result.Certificates) != 2 || len(result.Profiles) != 1 {
		t.Fatalf("result = %+v", result)
	}
	profile := result.Profiles[0]
	if profile.UUID != importProfileUUID || profile.BundleID != "com.example.app" || profile.ProfileType != "IOS_APP_STORE" || profile.TeamID != "TEAM123" {
		t.Fatalf("profile = %+v", profile)
	}

	wantTree := []string{
		"README",
		"certs/development/CA.cer.enc",
		"certs/distribution/65.cer.enc",
		"identities/development/" + fixture.developmentSHA + ".p12.enc",
		"identities/distribution/" + fixture.distributionSHA + ".p12.enc",
		"identity-contexts/",
		"profiles/appstore/App Store com.example.app.mobileprovision.enc",
	}
	tree := remoteTree(t, remotePath)
	if len(tree) != len(wantTree) {
		t.Fatalf("remote tree = %v, want %v", tree, wantTree)
	}
	for index, want := range wantTree {
		if !strings.HasPrefix(tree[index], want) {
			t.Fatalf("remote tree = %v, want %v", tree, wantTree)
		}
	}

	store := &signingpkg.GitStore{RepoURL: remoteURL, LocalDir: filepath.Join(t.TempDir(), "check"), Branch: "main"}
	t.Cleanup(func() { _ = store.Cleanup() })
	if err := store.Clone(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	encryptedFiles, err := store.ListEncryptedFiles()
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := decryptAndValidateSigningFiles(store, encryptedFiles, importDestinationPassword)
	if err != nil {
		t.Fatalf("validate imported store: %v", err)
	}
	selected, _, err := selectSigningPullFiles(decrypted, []string{"com.example.app"}, "IOS_APP_STORE")
	if err != nil {
		t.Fatalf("select imported files: %v", err)
	}
	identityPath := "identities/distribution/" + fixture.distributionSHA + ".p12"
	found := false
	for _, file := range selected {
		if canonicalSigningPullPath(file.RelativePath) == identityPath {
			found = true
			key, certificate, err := modernpkcs12.Decode(file.Plaintext, importDestinationPassword)
			if err != nil || !certificate.Equal(fixture.distribution) || !publicKeysEqual(key, fixture.distribution.PublicKey) {
				t.Fatalf("imported identity does not hold the source key pair: %v", err)
			}
		}
	}
	if !found {
		t.Fatalf("pull selection %v does not deliver %s", signingPullRelativePathsFromFiles(selected), identityPath)
	}

	stdout, stderr, err = runSigningSyncImport(t, args...)
	if err != nil {
		t.Fatalf("repeat import: %v\nstderr=%s", err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || result.PublicationState != "not-needed" {
		t.Fatalf("repeat import result = %+v (%v)", result, err)
	}
	if !reflect.DeepEqual(remoteTree(t, remotePath), tree) {
		t.Fatal("repeat import changed the destination")
	}
}

func TestSigningSyncImportDryRunListsWithoutDestination(t *testing.T) {
	fixture := newOpenSSLImportFixture(t)
	t.Setenv(signingSyncPasswordEnvVar, "")
	stdout, stderr, err := runSigningSyncImport(t, "--source", fixture.dir, "--source-password", importSourcePassword,
		"--repo", "file:///nonexistent/signing.git", "--dry-run", "--output", "json")
	if err != nil {
		t.Fatalf("dry run: %v\nstderr=%s", err, stderr)
	}
	var result asc.SigningSyncImportResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.PublicationState != "skipped" || result.RepoURL != "" {
		t.Fatalf("result = %+v", result)
	}
	gotCertificates := map[string]asc.SigningSyncImportCertificate{}
	for _, certificate := range result.Certificates {
		gotCertificates[certificate.SHA256] = certificate
	}
	if c := gotCertificates[fixture.distributionSHA]; c.Type != "distribution" || c.TeamID != "TEAM123" || c.ExpirationDate != fixture.distribution.NotAfter.UTC().Format(time.RFC3339) {
		t.Fatalf("distribution certificate = %+v", c)
	}
	if c := gotCertificates[fixture.developmentSHA]; c.Type != "development" {
		t.Fatalf("development certificate = %+v", c)
	}
	if len(result.Profiles) != 1 || result.Profiles[0].Name != "App Store com.example.app" || result.Profiles[0].UUID != importProfileUUID {
		t.Fatalf("profiles = %+v", result.Profiles)
	}
	if len(result.Files) != 6 {
		t.Fatalf("planned files = %v", result.Files)
	}
}

func TestSigningSyncImportFailuresWriteNothing(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, fixture openSSLImportFixture)
		pass    string
		wantErr string
	}{
		{
			name:    "wrong source password",
			pass:    "not-the-password",
			wantErr: "decrypt certs/development/CERT2.cer: wrong password or not an OpenSSL-encrypted signing file",
		},
		{
			name: "private key does not match certificate",
			mutate: func(t *testing.T, fixture openSSLImportFixture) {
				keyDER, err := x509.MarshalPKCS8PrivateKey(mustRSAKey(t))
				if err != nil {
					t.Fatal(err)
				}
				writePrivateTestFile(t, filepath.Join(fixture.dir, "certs", "distribution", "CERT1.p12"),
					encryptOpenSSLV1(t, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))
			},
			wantErr: "private key certs/distribution/CERT1.p12 does not match certificate certs/distribution/CERT1.cer",
		},
		{
			name: "symlink escaping the source",
			mutate: func(t *testing.T, fixture openSSLImportFixture) {
				outside := filepath.Join(t.TempDir(), "outside.cer")
				writePrivateTestFile(t, outside, encryptOpenSSLV1(t, fixture.distribution.Raw))
				link := filepath.Join(fixture.dir, "certs", "distribution", "CERT1.cer")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, link); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "read certs/distribution/CERT1.cer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOpenSSLImportFixture(t)
			if test.mutate != nil {
				test.mutate(t, fixture)
			}
			password := importSourcePassword
			if test.pass != "" {
				password = test.pass
			}
			remoteURL, remotePath := newSigningSyncBareRemote(t)
			before := remoteTree(t, remotePath)
			passwordFile := filepath.Join(t.TempDir(), "password")
			writePrivateTestFile(t, passwordFile, []byte(importDestinationPassword))
			_, _, err := runSigningSyncImport(t, "--source", fixture.dir, "--source-password", password,
				"--repo", remoteURL, "--password-file", passwordFile, "--confirm")
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
			if !reflect.DeepEqual(remoteTree(t, remotePath), before) {
				t.Fatal("failed import changed the destination")
			}
		})
	}
}

func TestSigningSyncImportFlagValidation(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"--source-password", "x", "--dry-run"}, wantErr: "--source is required"},
		{args: []string{"--source", ".", "--dry-run"}, wantErr: "--source-password is required"},
		{args: []string{"--source", ".", "--source-password", "x", "--repo", "r"}, wantErr: "--confirm is required to write the destination store (or pass --dry-run to preview)"},
		{args: []string{"--source", ".", "--source-password", "x", "--repo", "r", "--confirm", "--dry-run"}, wantErr: "--confirm and --dry-run are mutually exclusive"},
	}
	for _, test := range tests {
		t.Run(test.wantErr, func(t *testing.T) {
			_, _, err := runSigningSyncImport(t, test.args...)
			if !errors.Is(err, flag.ErrHelp) || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want usage error %q", err, test.wantErr)
			}
		})
	}
}
