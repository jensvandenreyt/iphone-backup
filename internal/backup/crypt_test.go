package backup

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestDecryptStream(t *testing.T) {
	key := randBytes(32)
	for _, n := range []int{0, 15, 16, 17, 65535, 65536, 65552, 200003} {
		plain := randBytes(n)
		enc := encryptCBC(t, key, plain)
		for _, extra := range []int64{0, 4096} {
			t.Run(fmt.Sprintf("len=%d/manifestExtra=%d", n, extra), func(t *testing.T) {
				var out bytes.Buffer
				written, err := decryptStream(&out, bytes.NewReader(enc), key, int64(n)+extra)
				if err != nil {
					t.Fatal(err)
				}
				if written != int64(n) || !bytes.Equal(out.Bytes(), plain) {
					t.Errorf("written = %d, got %d bytes, want %d matching bytes", written, out.Len(), n)
				}
			})
		}
	}
}

func TestDecryptStreamWithoutPaddingUsesManifestSize(t *testing.T) {
	key := randBytes(32)
	plain := randBytes(65536)
	enc := encryptCBC(t, key, plain)[:65536] // drop the padding block
	var out bytes.Buffer
	if _, err := decryptStream(&out, bytes.NewReader(enc), key, 65536); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), plain) {
		t.Error("content mismatch")
	}
}

func TestDecryptStreamDetectsTruncation(t *testing.T) {
	key := randBytes(32)
	// Fixed content: random bytes ending in 0x01 at a cut point would look like valid padding.
	enc := encryptCBC(t, key, bytes.Repeat([]byte("a"), 100000))
	for _, cut := range []int{0, 7, 65536, 65536 + 16} {
		_, err := decryptStream(&bytes.Buffer{}, bytes.NewReader(enc[:cut]), key, 100000)
		if !errors.Is(err, errTruncated) {
			t.Errorf("cut at %d: err = %v, want errTruncated", cut, err)
		}
	}
}
