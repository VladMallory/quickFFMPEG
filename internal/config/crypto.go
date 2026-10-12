package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

func LoadKeys() (domain.Crypt, domain.Crypt, error) {
	dec, err := loadCrypt(domain.EnvKey, domain.EnvIV)
	if err != nil {
		return domain.Crypt{}, domain.Crypt{}, err
	}
	enc, err := loadCrypt(domain.EnvOKey, domain.EnvOIV)
	if err != nil {
		return domain.Crypt{}, domain.Crypt{}, err
	}
	return dec, enc, nil
}

func loadCrypt(keyName, ivName string) (domain.Crypt, error) {
	c := domain.Crypt{Key: os.Getenv(keyName), IV: os.Getenv(ivName)}
	if c.Key == "" || c.IV == "" {
		return domain.Crypt{}, fmt.Errorf("не заданы переменные %s и %s", keyName, ivName)
	}
	if !isAES128Hex(c.Key) {
		return domain.Crypt{}, fmt.Errorf("%s должен быть 32 hex-символа (AES-128)", keyName)
	}
	if !isAES128Hex(c.IV) {
		return domain.Crypt{}, fmt.Errorf("%s должен быть 32 hex-символа", ivName)
	}
	return c, nil
}

func isAES128Hex(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 16
}
