package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

type FileCipher struct{}

func New() *FileCipher {
	return &FileCipher{}
}

func (FileCipher) RandHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (FileCipher) EncryptFile(in, out, keyHex, ivHex string) error {
	key, iv, err := parseKeyIV(keyHex, ivHex)
	if err != nil {
		return err
	}
	plain, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	padded := pkcs7Pad(plain, aes.BlockSize)
	data := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(data, padded)
	return os.WriteFile(out, data, 0o600)
}

func (FileCipher) DecryptFile(in, out, keyHex, ivHex string) error {
	key, iv, err := parseKeyIV(keyHex, ivHex)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return fmt.Errorf("битый шифротекст: размер не кратен блоку")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return err
	}
	return os.WriteFile(out, plain, 0o644)
}

func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func parseKeyIV(keyHex, ivHex string) ([]byte, []byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 16 {
		return nil, nil, fmt.Errorf("ключ должен быть 32 hex-символа (AES-128)")
	}
	iv, err := hex.DecodeString(ivHex)
	if err != nil || len(iv) != 16 {
		return nil, nil, fmt.Errorf("iv должен быть 32 hex-символа")
	}
	return key, iv, nil
}

func pkcs7Pad(data []byte, block int) []byte {
	pad := block - len(data)%block
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("битый шифротекст: пустой блок")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(data) {
		return nil, fmt.Errorf("битый шифротекст: неверный паддинг")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, fmt.Errorf("битый шифротекст: неверный паддинг")
		}
	}
	return data[:len(data)-pad], nil
}
