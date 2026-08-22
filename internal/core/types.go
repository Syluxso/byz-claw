package core

// LoopConfig holds knobs for the fixed agent loop.
type LoopConfig struct {
	MaxToolIterations     int
	CompactTokenThreshold int
	CompactTailMessages   int
	TokenCeiling          int
	Profile               string // interactive | heartbeat
}

func DefaultLoopConfig() LoopConfig {
	return LoopConfig{
		MaxToolIterations:     12,
		CompactTokenThreshold: 8000,
		CompactTailMessages:   10,
		TokenCeiling:          0,
		Profile:               "interactive",
	}
}
