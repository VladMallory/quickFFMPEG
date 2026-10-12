package domain

import "errors"

var (
	ErrUnknownCodec      = errors.New("неизвестный кодек")
	ErrUnknownAudioCodec = errors.New("неизвестный аудиокодек")
)

var ValidExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".webm": true,
}
