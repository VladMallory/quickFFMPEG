package domain

type FileStat struct {
	Name             string
	OutputPath       string
	SizeBefore       float64
	SizeAfter        float64
	ResolutionBefore string
	ResolutionAfter  string
}
