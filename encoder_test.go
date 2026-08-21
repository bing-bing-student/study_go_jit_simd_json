package jitjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestStaticEncoderModes(t *testing.T) {
	batch := sampleBatch()
	want, err := MarshalReference(batch)
	if err != nil {
		t.Fatal(err)
	}

	modes := []Mode{ModeScalar, ModeAuto}
	if SupportsAVX2() {
		modes = append(modes, ModeAVX2)
	}
	for _, mode := range modes {
		t.Run(modeName(mode), func(t *testing.T) {
			encoder, err := NewEncoder(Options{Mode: mode, Backend: BackendStatic})
			if err != nil {
				t.Fatalf("NewEncoder: %v", err)
			}
			defer encoder.Close()

			got, err := encoder.Marshal(batch)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
			}
			if !json.Valid(got) {
				t.Fatalf("invalid JSON: %s", got)
			}
		})
	}
}

func TestMarshalPacked(t *testing.T) {
	batch := sampleBatch()
	packed, err := Pack(batch)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	got, err := encoder.MarshalPacked(packed)
	if err != nil {
		t.Fatal(err)
	}
	want, err := MarshalReference(batch)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestNilAndEmptyCollections(t *testing.T) {
	tests := []OrderBatch{
		{},
		{Orders: []Order{}},
		{Orders: []Order{{Items: nil}}},
		{Orders: []Order{{Items: []Item{}}}},
	}
	encoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	for _, batch := range tests {
		want, err := json.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		got, err := encoder.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
		}
	}
}

func TestEncoderClose(t *testing.T) {
	encoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Marshal(OrderBatch{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Marshal error = %v, want ErrClosed", err)
	}
	if err := encoder.Close(); !errors.Is(err, ErrClosed) {
		t.Fatalf("second Close error = %v, want ErrClosed", err)
	}
}

func TestTrustUTF8(t *testing.T) {
	batch := OrderBatch{Orders: []Order{{OrderID: string([]byte{0xff}), Items: []Item{}}}}

	strict, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	defer strict.Close()
	if _, err := strict.Marshal(batch); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("strict Marshal error = %v, want ErrInvalidUTF8", err)
	}

	trusted, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic, TrustUTF8: true})
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Close()
	output, err := trusted.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte{0xff}) {
		t.Fatal("trusted output must preserve the caller-provided bytes")
	}
}

func TestScratchReuseResetsConditionalFields(t *testing.T) {
	tracking := "TRACK-1"
	encoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT})
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	batches := []OrderBatch{
		{Orders: []Order{{Payment: Payment{Paid: true}, Shipping: Shipping{TrackingNo: &tracking}, Items: nil}}},
		{Orders: []Order{{Payment: Payment{Paid: false}, Shipping: Shipping{TrackingNo: nil}, Items: []Item{}}}},
	}
	for iteration := 0; iteration < 20; iteration++ {
		for _, batch := range batches {
			want, err := json.Marshal(batch)
			if err != nil {
				t.Fatal(err)
			}
			got, err := encoder.Marshal(batch)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
			}
		}
	}
}

func modeName(mode Mode) string {
	switch mode {
	case ModeAuto:
		return "auto"
	case ModeScalar:
		return "scalar"
	case ModeAVX2:
		return "avx2"
	default:
		return "unknown"
	}
}

func TestJITEncoderModes(t *testing.T) {
	batch := sampleBatch()
	want, err := MarshalReference(batch)
	if err != nil {
		t.Fatal(err)
	}

	modes := []Mode{ModeScalar, ModeAuto}
	if SupportsAVX2() {
		modes = append(modes, ModeAVX2)
	}
	for _, mode := range modes {
		t.Run(modeName(mode), func(t *testing.T) {
			encoder, err := NewEncoder(Options{Mode: mode, Backend: BackendJIT})
			if err != nil {
				t.Fatalf("NewEncoder: %v", err)
			}
			defer encoder.Close()

			got, err := encoder.Marshal(batch)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestJITCodeSize(t *testing.T) {
	jitEncoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendJIT})
	if err != nil {
		t.Fatal(err)
	}
	defer jitEncoder.Close()
	if jitEncoder.CodeSize() == 0 {
		t.Fatal("JIT encoder must contain generated machine code")
	}

	staticEncoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	defer staticEncoder.Close()
	if staticEncoder.CodeSize() != 0 {
		t.Fatalf("static encoder code size = %d, want 0", staticEncoder.CodeSize())
	}
}

func TestConcurrentMarshal(t *testing.T) {
	encoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT})
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	batch := sampleBatch()
	want, err := MarshalReference(batch)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errorsChannel := make(chan error, 16)
	for worker := 0; worker < 16; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 50; iteration++ {
				got, marshalErr := encoder.Marshal(batch)
				if marshalErr != nil {
					errorsChannel <- marshalErr
					return
				}
				if !bytes.Equal(got, want) {
					errorsChannel <- errors.New("concurrent output mismatch")
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for workerErr := range errorsChannel {
		t.Error(workerErr)
	}
}
