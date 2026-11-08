package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
)

var zeroIV = make([]byte, aes.BlockSize)

var errTruncated = errors.New("encrypted file is truncated")

func decryptCBC(data, key []byte) ([]byte, error) {
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the AES block size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, zeroIV).CryptBlocks(out, data)
	return out, nil
}

// decryptStream decrypts AES-256-CBC data from src and writes the first size bytes of plaintext to dst.
// When the ciphertext ends before size bytes but its last block is valid PKCS#7 padding, the file is
// complete and size is stale, which happens when a file changes while the backup runs.
// It returns the number of plaintext bytes written.
func decryptStream(dst io.Writer, src io.Reader, key []byte, size int64) (int64, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	mode := cipher.NewCBCDecrypter(block, zeroIV)
	var written int64
	emit := func(p []byte) error {
		p = p[:min(int64(len(p)), size-written)]
		written += int64(len(p))
		_, err := dst.Write(p)
		return err
	}
	buf := make([]byte, 64*1024)
	var last []byte // the last decrypted block so far, held back because it may be padding
	truncated := func() error {
		return fmt.Errorf("%w: decrypted %d of %d bytes", errTruncated, written+int64(len(last)), size)
	}
	for written < size {
		n, err := io.ReadFull(src, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return written, err
		}
		if n%aes.BlockSize != 0 {
			return written, truncated()
		}
		if n == 0 {
			if written+int64(len(last)) < size {
				pad := pkcs7Len(last)
				if pad == 0 {
					return written, truncated()
				}
				last = last[:len(last)-pad]
			}
			err := emit(last)
			return written, err
		}
		mode.CryptBlocks(buf[:n], buf[:n])
		if err := emit(last); err != nil {
			return written, err
		}
		if err := emit(buf[:n-aes.BlockSize]); err != nil {
			return written, err
		}
		last = append(last[:0], buf[n-aes.BlockSize:n]...)
	}
	return written, nil
}

// pkcs7Len returns the padding length of a final PKCS#7-padded block, or 0 when the padding is invalid.
func pkcs7Len(block []byte) int {
	if len(block) != aes.BlockSize {
		return 0
	}
	n := int(block[len(block)-1])
	if n == 0 || n > aes.BlockSize {
		return 0
	}
	for _, b := range block[len(block)-n:] {
		if int(b) != n {
			return 0
		}
	}
	return n
}
