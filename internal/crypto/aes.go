package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher((key))
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal appends the encrypted data and auth tag to the nonce
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil

}

func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, encryptedText := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Open authenticates and decrypts the ciphertext
	plaintext, err := aesGCM.Open(nil, nonce, encryptedText, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
