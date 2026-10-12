package crypt

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.bin")
	enc := filepath.Join(dir, "enc.bin")
	back := filepath.Join(dir, "back.bin")

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i * 7)
	}
	if err := os.WriteFile(plain, data, 0o644); err != nil {
		t.Fatal(err)
	}

	c := New()
	key, err := c.RandHex(16)
	if err != nil {
		t.Fatal(err)
	}
	iv, err := c.RandHex(16)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EncryptFile(plain, enc, key, iv); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(enc)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size()%16 != 0 {
		t.Fatalf("размер не кратен блоку: %d", info.Size())
	}
	if err := c.DecryptFile(enc, back, key, iv); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != hex.EncodeToString(data) {
		t.Fatal("расшифровка не совпала с оригиналом")
	}
}

func TestOpenSSLVector(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.bin")
	enc := filepath.Join(dir, "enc.bin")
	back := filepath.Join(dir, "back.bin")

	if err := os.WriteFile(plain, []byte("hello quickffmpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New()
	key := "000102030405060708090a0b0c0d0e0f"
	iv := "0f0e0d0c0b0a09080706050403020100"
	want := "fc7b323eaec0c6e8ad60ad53c99fbbe211d39de30795584c0228e5abc391e925"

	if err := c.EncryptFile(plain, enc, key, iv); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != want {
		t.Fatalf("шифротекст не совпал с openssl: %x", got)
	}
	if err := c.DecryptFile(enc, back, key, iv); err != nil {
		t.Fatal(err)
	}
}
