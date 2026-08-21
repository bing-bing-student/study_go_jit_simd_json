//go:build !linux || !amd64 || !cgo

package native

import "errors"

type Mode uint8

const (
	ModeAuto Mode = iota
	ModeScalar
	ModeAVX2
)

type Backend uint8

const (
	BackendJIT Backend = iota
	BackendStatic
)

type Encoder struct{}

var errUnsupported = errors.New("native encoder requires linux/amd64 with cgo enabled")

func NewEncoder(Mode, Backend) (*Encoder, error) {
	return nil, errUnsupported
}

func (*Encoder) Encode([]OrderRow, bool, []ItemRow, []byte, []byte) (int, error) {
	return 0, errUnsupported
}

func (*Encoder) Close() {}

func (*Encoder) CodeSize() int { return 0 }

func SupportsAVX2() bool { return false }
