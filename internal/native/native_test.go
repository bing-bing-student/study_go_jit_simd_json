//go:build linux && amd64 && cgo

package native

import "testing"

func TestEncodeRejectsSmallOutput(t *testing.T) {
	encoder, err := NewEncoder(ModeScalar, BackendStatic)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	_, err = encoder.Encode(nil, false, nil, nil, false, make([]byte, 1))
	status, ok := err.(*StatusError)
	if !ok || status.Status != StatusNoSpace {
		t.Fatalf("Encode error = %v, want status %d", err, StatusNoSpace)
	}
}

func TestEncodeRejectsInvalidStringRef(t *testing.T) {
	encoder, err := NewEncoder(ModeScalar, BackendStatic)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	orders := []OrderRow{{
		OrderID:   StringRef{Offset: 1, Length: 1},
		ItemsNull: 1,
	}}
	_, err = encoder.Encode(orders, false, nil, nil, false, make([]byte, 1024))
	status, ok := err.(*StatusError)
	if !ok || status.Status != StatusInvalidArgument {
		t.Fatalf("Encode error = %v, want status %d", err, StatusInvalidArgument)
	}
}
