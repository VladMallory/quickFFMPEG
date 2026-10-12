package domain

const (
	EnvKey  = "QFF_KEY"
	EnvIV   = "QFF_IV"
	EnvOKey = "QFF_OKEY"
	EnvOIV  = "QFF_OIV"
)

type Crypt struct {
	Key string
	IV  string
}
