package jitjson

import "errors"

var (
	ErrClosed         = errors.New("jitjson: encoder is closed")
	ErrNilPacked      = errors.New("jitjson: packed batch is nil")
	ErrInvalidUTF8    = errors.New("jitjson: input contains invalid UTF-8")
	ErrOutputTooLarge = errors.New("jitjson: output size exceeds platform limit")
	ErrPackedTooLarge = errors.New("jitjson: packed data exceeds uint32 offsets")
	ErrUnsupportedCPU = errors.New("jitjson: requested CPU feature is unavailable")
	ErrInvalidMode    = errors.New("jitjson: invalid mode or backend")
	ErrNative         = errors.New("jitjson: native encoder failed")
)
