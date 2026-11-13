package defaults

import "time"

const (
	CleanIdlePeriod       = 10 * time.Second
	ControlLoopInterval   = 60 * time.Second
	ControlLoopMaxJitter  = 1000 * time.Millisecond
	OpaStartupTimeout     = 30 * time.Second
	MaxRequestBodySize    = 1 << 20 // 1 MB
	MaxMemoryBodyParsing  = 100 * MaxRequestBodySize
	RequestBodyBufferSize = 8 * 1024 // 8 KB
)
