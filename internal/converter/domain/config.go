package domain

type Config struct {
	Codec           string `json:"codec"`
	Path            string `json:"path"`
	CRF             string `json:"crf"`
	Resize          int    `json:"resize"`
	Preset          string `json:"preset"`
	AudioCodec      string `json:"audio_codec"`
	AudioCodecValue string `json:"audio_codecValue"`
	AudioBitrate    string `json:"audio_bitrate"`
	MaxFps          int    `json:"maxFps"`
	PerFps          bool   `json:"perFps"`
	FilmGrain       int    `json:"film_grain"`
	BitDepth        int    `json:"bit_depth"`
}

func (c *Config) ApplyDefaults() error {

	if c.Codec != "av1" && c.Codec != "265" {
		return ErrUnknownCodec
	}

	if c.AudioCodec != "aac" && c.AudioCodec != "opus" {
		return ErrUnknownAudioCodec
	}

	switch c.AudioCodec {
	case "opus":
		c.AudioCodecValue = "libopus"
		c.AudioBitrate = "96k"
	case "aac":
		c.AudioCodecValue = "aac"
		c.AudioBitrate = "128k"
	}

	if c.Preset == "" {
		if c.Codec == "265" {
			c.Preset = "slow"
		} else {
			c.Preset = "1"
		}
	}

	return nil
}
