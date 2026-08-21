package jitjson

import (
	"errors"
	"fmt"
	"sync"

	"github.com/bytedance/gopkg/lang/dirtmake"

	"github.com/bing-bing-student/study_go_jit_simd_json/internal/native"
)

type Options struct {
	Mode    Mode
	Backend Backend

	// TrustUTF8 skips input validation. Every string must already contain valid UTF-8.
	TrustUTF8 bool
}

const maxRetainedPackedBytes = 64 << 20

type Encoder struct {
	mutex     sync.RWMutex
	native    *native.Encoder
	closed    bool
	trustUTF8 bool
	scratch   sync.Pool
}

func NewEncoder(options Options) (*Encoder, error) {
	if options.Mode > ModeAVX2 || options.Backend > BackendStatic {
		return nil, ErrInvalidMode
	}
	encoder, err := native.NewEncoder(native.Mode(options.Mode), native.Backend(options.Backend))
	if err != nil {
		return nil, mapNativeError(err)
	}
	return &Encoder{native: encoder, trustUTF8: options.TrustUTF8}, nil
}

func (e *Encoder) Marshal(batch OrderBatch) ([]byte, error) {
	if e == nil {
		return nil, ErrClosed
	}

	var packed *PackedBatch
	if value := e.scratch.Get(); value != nil {
		packed = value.(*PackedBatch)
	} else {
		packed = &PackedBatch{}
	}
	defer e.releasePacked(packed)

	if err := packIntoReusable(batch, packed, !e.trustUTF8); err != nil {
		return nil, err
	}
	return e.MarshalPacked(packed)
}

func (e *Encoder) releasePacked(packed *PackedBatch) {
	if packed.retainedBytes() > maxRetainedPackedBytes {
		return
	}
	packed.orders = packed.orders[:0]
	packed.items = packed.items[:0]
	packed.strings = packed.strings[:0]
	e.scratch.Put(packed)
}

func (e *Encoder) MarshalPacked(batch *PackedBatch) ([]byte, error) {
	if batch == nil {
		return nil, ErrNilPacked
	}
	if e == nil {
		return nil, ErrClosed
	}

	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if e.closed || e.native == nil {
		return nil, ErrClosed
	}

	capacity := batch.maxOutput
	if capacity < 1 {
		capacity = 1
	}
	output := dirtmake.Bytes(capacity, capacity)
	written, err := e.native.Encode(batch.orders, batch.ordersNull, batch.items, batch.strings, batch.stringsPlain, output)
	if err != nil {
		return nil, mapNativeError(err)
	}
	// Pack computes the exact size, so equality also guarantees that no uninitialized byte is exposed.
	if written != len(output) {
		return nil, fmt.Errorf("%w: invalid written length %d, want %d", ErrNative, written, len(output))
	}
	return output, nil
}

func (e *Encoder) CodeSize() int {
	if e == nil {
		return 0
	}
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if e.closed || e.native == nil {
		return 0
	}
	return e.native.CodeSize()
}

func (e *Encoder) Close() error {
	if e == nil {
		return ErrClosed
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.closed {
		return ErrClosed
	}
	e.native.Close()
	e.native = nil
	e.closed = true
	return nil
}

func SupportsAVX2() bool {
	return native.SupportsAVX2()
}

func mapNativeError(err error) error {
	var status *native.StatusError
	if !errors.As(err, &status) {
		return fmt.Errorf("%w: %v", ErrNative, err)
	}
	switch status.Status {
	case native.StatusUnsupportedCPU:
		return fmt.Errorf("%w: %s", ErrUnsupportedCPU, status.Message)
	default:
		return fmt.Errorf("%w: %s", ErrNative, status.Message)
	}
}
