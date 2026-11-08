package backup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"howett.net/plist"
)

const (
	testDomain   = "AppDomain-com.example.test"
	testPath     = "Documents/hello.txt"
	testPassword = "correct horse"
	testClass    = 3
)

var testContent = []byte("hello world, this is not a multiple of sixteen bytes")

func TestAESUnwrapRFC3394Vectors(t *testing.T) {
	tests := []struct{ kek, key, wrapped string }{
		{
			"000102030405060708090A0B0C0D0E0F",
			"00112233445566778899AABBCCDDEEFF",
			"1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5",
		},
		{
			"000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F",
			"00112233445566778899AABBCCDDEEFF000102030405060708090A0B0C0D0E0F",
			"28C9F404C4B810F4CBCCB35CFB87F8263F5786E2D80ED326CBC7F0E71A99F43BFB988B9B7A02DD21",
		},
	}
	for _, tt := range tests {
		got, err := aesUnwrap(mustHex(t, tt.kek), mustHex(t, tt.wrapped))
		if err != nil {
			t.Fatalf("unwrap: %v", err)
		}
		if !bytes.Equal(got, mustHex(t, tt.key)) {
			t.Errorf("unwrap = %x, want %s", got, tt.key)
		}
		if wrapped := aesWrap(mustHex(t, tt.kek), got); !bytes.Equal(wrapped, mustHex(t, tt.wrapped)) {
			t.Errorf("wrap = %x, want %s", wrapped, tt.wrapped)
		}
	}
}

func TestAESUnwrapWrongKeyFailsIntegrityCheck(t *testing.T) {
	wrapped := mustHex(t, "1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5")
	if _, err := aesUnwrap(make([]byte, 16), wrapped); !errors.Is(err, errUnwrap) {
		t.Fatalf("err = %v, want errUnwrap", err)
	}
}

func TestOpenDetectsEncryption(t *testing.T) {
	for _, encrypted := range []bool{true, false} {
		b, err := Open(buildBackup(t, encrypted))
		if err != nil {
			t.Fatal(err)
		}
		if b.Encrypted != encrypted {
			t.Errorf("Encrypted = %v, want %v", b.Encrypted, encrypted)
		}
		if b.Info.DeviceName != "Test iPhone" {
			t.Errorf("DeviceName = %q", b.Info.DeviceName)
		}
	}
}

func TestUnlockWithWrongPassword(t *testing.T) {
	b, err := Open(buildBackup(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock("nope"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("err = %v, want ErrWrongPassword", err)
	}
}

func TestExtractFile(t *testing.T) {
	for _, encrypted := range []bool{true, false} {
		b, err := Open(buildBackup(t, encrypted))
		if err != nil {
			t.Fatal(err)
		}
		if err := b.Unlock(testPassword); err != nil {
			t.Fatalf("unlock: %v", err)
		}
		t.Cleanup(func() { b.Close() })
		f, err := b.FindFile(testDomain, testPath)
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		var out bytes.Buffer
		if err := b.Extract(f, &out); err != nil {
			t.Fatalf("extract: %v", err)
		}
		if !bytes.Equal(out.Bytes(), testContent) {
			t.Errorf("encrypted=%v: content = %q", encrypted, out.Bytes())
		}
	}
}

func TestFindFileNotFound(t *testing.T) {
	b, err := Open(buildBackup(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	if _, err := b.FindFile(testDomain, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCloseRemovesTempDir(t *testing.T) {
	b, err := Open(buildBackup(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	tmp := b.TempPath("")
	b.Close()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp dir still exists: %v", err)
	}
}

// buildBackup writes a minimal backup with one file, mirroring the layout iOS uses.
func buildBackup(t *testing.T, encrypted bool) string {
	t.Helper()
	dir := t.TempDir()
	classKey, manifestKey, fileKey := randBytes(32), randBytes(32), randBytes(32)

	content := testContent
	var encKey []byte
	if encrypted {
		content = encryptCBC(t, fileKey, testContent)
		encKey = append(le32(testClass), aesWrap(classKey, fileKey)...)
	}
	sum := sha1.Sum([]byte(testDomain + "-" + testPath))
	fileID := hex.EncodeToString(sum[:])
	writeFile(t, filepath.Join(dir, fileID[:2], fileID), content)

	db := buildManifestDB(t, fileID, mbFile(t, int64(len(testContent)), encKey))
	manifest := map[string]any{"IsEncrypted": encrypted}
	if encrypted {
		db = encryptCBC(t, manifestKey, db)
		manifest["BackupKeyBag"] = buildKeybag(t, classKey)
		manifest["ManifestKey"] = append(le32(testClass), aesWrap(classKey, manifestKey)...)
	}
	writeFile(t, filepath.Join(dir, "Manifest.db"), db)
	writePlist(t, filepath.Join(dir, "Manifest.plist"), manifest)
	writePlist(t, filepath.Join(dir, "Info.plist"), map[string]any{
		"Device Name":     "Test iPhone",
		"Product Version": "18.0",
		"Product Type":    "iPhone15,2",
	})
	return dir
}

func buildKeybag(t *testing.T, classKey []byte) []byte {
	salt, dpsl := randBytes(20), randBytes(20)
	const iter, dpic = 10, 10
	pw, err := pbkdf2.Key(sha256.New, testPassword, dpsl, dpic, 32)
	if err != nil {
		t.Fatal(err)
	}
	passcodeKey, err := pbkdf2.Key(sha1.New, string(pw), salt, iter, 32)
	if err != nil {
		t.Fatal(err)
	}
	var kb bytes.Buffer
	tlv := func(tag string, val []byte) {
		kb.WriteString(tag)
		kb.Write(be32(uint32(len(val))))
		kb.Write(val)
	}
	tlv("VERS", be32(4))
	tlv("TYPE", be32(1))
	tlv("UUID", randBytes(16))
	tlv("WRAP", be32(0))
	tlv("SALT", salt)
	tlv("ITER", be32(iter))
	tlv("DPWT", be32(1))
	tlv("DPIC", be32(dpic))
	tlv("DPSL", dpsl)
	tlv("UUID", randBytes(16))
	tlv("CLAS", be32(testClass))
	tlv("WRAP", be32(wrapPasscode))
	tlv("KTYP", be32(0))
	tlv("WPKY", aesWrap(passcodeKey, classKey))
	return kb.Bytes()
}

func buildManifestDB(t *testing.T, fileID string, blob []byte) []byte {
	path := filepath.Join(t.TempDir(), "Manifest.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE Files (fileID TEXT PRIMARY KEY, domain TEXT, relativePath TEXT, flags INTEGER, file BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO Files VALUES (?, ?, ?, 1, ?)`, fileID, testDomain, testPath, blob); err != nil {
		t.Fatal(err)
	}
	db.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mbFile(t *testing.T, size int64, encKey []byte) []byte {
	root := map[string]any{"Size": size, "ProtectionClass": testClass, "$class": plist.UID(2)}
	objects := []any{"$null", root, map[string]any{"$classname": "MBFile", "$classes": []any{"MBFile", "NSObject"}}}
	if encKey != nil {
		root["EncryptionKey"] = plist.UID(3)
		objects = append(objects, map[string]any{"NS.data": encKey})
	}
	data, err := plist.Marshal(map[string]any{
		"$version":  100000,
		"$archiver": "NSKeyedArchiver",
		"$top":      map[string]any{"root": plist.UID(1)},
		"$objects":  objects,
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func aesWrap(kek, key []byte) []byte {
	block, err := aes.NewCipher(kek)
	if err != nil {
		panic(err)
	}
	n := len(key) / 8
	a := append([]byte(nil), defaultIV...)
	r := append([]byte(nil), key...)
	buf := make([]byte, 16)
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			copy(buf[:8], a)
			copy(buf[8:], r[(i-1)*8:i*8])
			block.Encrypt(buf, buf)
			binary.BigEndian.PutUint64(a, binary.BigEndian.Uint64(buf[:8])^uint64(n*j+i))
			copy(r[(i-1)*8:i*8], buf[8:])
		}
	}
	return append(a, r...)
}

func encryptCBC(t *testing.T, key, data []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	pad := aes.BlockSize - len(data)%aes.BlockSize
	out := append(append([]byte(nil), data...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	cipher.NewCBCEncrypter(block, zeroIV).CryptBlocks(out, out)
	return out
}

func writeFile(t *testing.T, path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePlist(t *testing.T, path string, v any) {
	data, err := plist.Marshal(v, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, data)
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

func mustHex(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
