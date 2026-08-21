//go:build linux && amd64 && cgo

package native

/*
#cgo CFLAGS: -O3 -std=c11 -Wall -Wextra -D_GNU_SOURCE
#include "jitjson.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

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

type Encoder struct {
	pointer *C.jitjson_encoder_t
}

type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("native encoder: %s (status %d)", e.Message, e.Status)
}

func NewEncoder(mode Mode, backend Backend) (*Encoder, error) {
	if err := ValidateGoLayout(); err != nil {
		return nil, err
	}
	var pointer *C.jitjson_encoder_t
	status := C.jitjson_encoder_create(C.jitjson_mode_t(mode), C.jitjson_backend_t(backend), &pointer)
	if err := statusError(status); err != nil {
		return nil, err
	}
	return &Encoder{pointer: pointer}, nil
}

func (e *Encoder) Encode(
	orders []OrderRow,
	ordersNull bool,
	items []ItemRow,
	strings []byte,
	stringsPlain bool,
	output []byte,
) (int, error) {
	if e == nil || e.pointer == nil || len(output) == 0 {
		return 0, &StatusError{Status: int(C.JITJSON_ERR_INVALID_ARGUMENT), Message: "invalid argument"}
	}

	var written C.size_t
	var ordersPointer *C.jitjson_order_row_t
	var itemsPointer *C.jitjson_item_row_t
	var stringsPointer *C.uint8_t
	if len(orders) != 0 {
		ordersPointer = (*C.jitjson_order_row_t)(unsafe.Pointer(unsafe.SliceData(orders)))
	}
	if len(items) != 0 {
		itemsPointer = (*C.jitjson_item_row_t)(unsafe.Pointer(unsafe.SliceData(items)))
	}
	if len(strings) != 0 {
		stringsPointer = (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(strings)))
	}
	ordersNullValue := C.uint8_t(0)
	if ordersNull {
		ordersNullValue = 1
	}
	stringsPlainValue := C.uint8_t(0)
	if stringsPlain {
		stringsPlainValue = 1
	}

	status := C.jitjson_encoder_encode(
		e.pointer,
		ordersPointer,
		C.size_t(len(orders)),
		ordersNullValue,
		itemsPointer,
		C.size_t(len(items)),
		stringsPointer,
		C.size_t(len(strings)),
		stringsPlainValue,
		(*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(output))),
		C.size_t(len(output)),
		&written,
	)

	runtime.KeepAlive(orders)
	runtime.KeepAlive(items)
	runtime.KeepAlive(strings)
	runtime.KeepAlive(output)

	if err := statusError(status); err != nil {
		return int(written), err
	}
	return int(written), nil
}

func (e *Encoder) CodeSize() int {
	if e == nil || e.pointer == nil {
		return 0
	}
	return int(C.jitjson_encoder_code_size(e.pointer))
}

func (e *Encoder) Close() {
	if e == nil || e.pointer == nil {
		return
	}
	C.jitjson_encoder_destroy(e.pointer)
	e.pointer = nil
}

func SupportsAVX2() bool {
	return C.jitjson_cpu_supports_avx2() != 0
}

func statusError(status C.jitjson_status_t) error {
	if status == C.JITJSON_OK {
		return nil
	}
	return &StatusError{
		Status:  int(status),
		Message: C.GoString(C.jitjson_status_message(status)),
	}
}
