//go:build benchmark

package jitjson

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	goccyjson "github.com/goccy/go-json"
	jsoniter "github.com/json-iterator/go"
)

func TestExternalRealDataMarshalers(t *testing.T) {
	batch := loadRealData(t)
	expected, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}

	marshalers := []struct {
		name    string
		marshal func() ([]byte, error)
	}{
		{name: "jsoniter", marshal: func() ([]byte, error) { return jsoniter.Marshal(batch) }},
		{name: "goccy_go_json", marshal: func() ([]byte, error) { return goccyjson.Marshal(batch) }},
		{name: "sonic", marshal: func() ([]byte, error) { return sonic.Marshal(batch) }},
	}

	for _, marshaler := range marshalers {
		t.Run(marshaler.name, func(t *testing.T) {
			output, err := marshaler.marshal()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output, expected) {
				t.Fatalf("output differs from encoding/json: got %d bytes, want %d", len(output), len(expected))
			}
		})
	}
}

func BenchmarkRealDataMarshal(b *testing.B) {
	batch := loadRealData(b)
	expected, err := json.Marshal(batch)
	if err != nil {
		b.Fatal(err)
	}
	packed, err := Pack(batch)
	if err != nil {
		b.Fatal(err)
	}

	jitEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT})
	if err != nil {
		b.Fatal(err)
	}
	defer jitEncoder.Close()

	staticEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendStatic})
	if err != nil {
		b.Fatal(err)
	}
	defer staticEncoder.Close()

	scalarEncoder, err := NewEncoder(Options{Mode: ModeScalar, Backend: BackendJIT})
	if err != nil {
		b.Fatal(err)
	}
	defer scalarEncoder.Close()

	trustedEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT, TrustUTF8: true})
	if err != nil {
		b.Fatal(err)
	}
	defer trustedEncoder.Close()

	benchmarkMarshaler := func(name string, marshal func() ([]byte, error)) {
		b.Run(name, func(b *testing.B) {
			output, err := marshal()
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(output, expected) {
				b.Fatalf("output differs from encoding/json: got %d bytes, want %d", len(output), len(expected))
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(expected)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				output, err = marshal()
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOutput = output
			}
		})
	}

	benchmarkMarshaler("encoding_json", func() ([]byte, error) { return json.Marshal(batch) })
	benchmarkMarshaler("jsoniter", func() ([]byte, error) { return jsoniter.Marshal(batch) })
	benchmarkMarshaler("goccy_go_json", func() ([]byte, error) { return goccyjson.Marshal(batch) })
	benchmarkMarshaler("sonic", func() ([]byte, error) { return sonic.Marshal(batch) })

	b.Run("jitjson_pack_only", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(expected)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			value, packErr := Pack(batch)
			if packErr != nil {
				b.Fatal(packErr)
			}
			benchmarkPacked = value
		}
	})

	benchmarkMarshaler("jitjson_static_auto_packed", func() ([]byte, error) {
		return staticEncoder.MarshalPacked(packed)
	})
	benchmarkMarshaler("jitjson_jit_scalar_packed", func() ([]byte, error) {
		return scalarEncoder.MarshalPacked(packed)
	})
	benchmarkMarshaler("jitjson_jit_auto_packed", func() ([]byte, error) {
		return jitEncoder.MarshalPacked(packed)
	})
	benchmarkMarshaler("jitjson_jit_auto_with_pack", func() ([]byte, error) {
		return jitEncoder.Marshal(batch)
	})
	benchmarkMarshaler("jitjson_jit_auto_trusted_with_pack", func() ([]byte, error) {
		return trustedEncoder.Marshal(batch)
	})
}
