package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"io"
)

var zeroIV = make([]byte, aes.BlockSize)

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
func decryptStream(dst io.Writer, src io.Reader, key []byte, size int64) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	mode := cipher.NewCBCDecrypter(block, zeroIV)
	buf := make([]byte, 64*1024)
	for remaining := size; remaining > 0; {
		n, err := io.ReadFull(src, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return err
		}
		if n == 0 || n%aes.BlockSize != 0 {
			return errors.New("encrypted file is truncated")
		}
		mode.CryptBlocks(buf[:n], buf[:n])
		chunk := buf[:min(int64(n), remaining)]
		if _, err := dst.Write(chunk); err != nil {
			return err
		}
		remaining -= int64(len(chunk))
	}
	return nil
}
