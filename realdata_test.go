package jitjson

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

const realDataPath = "testdata/orders.json"

func loadRealData(tb testing.TB) OrderBatch {
	tb.Helper()

	data, err := os.ReadFile(realDataPath)
	if err != nil {
		tb.Fatalf("read %s: %v", realDataPath, err)
	}
	var batch OrderBatch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		tb.Fatalf("decode %s: %v", realDataPath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		tb.Fatalf("%s contains trailing JSON data", realDataPath)
	}
	if len(batch.Orders) == 0 {
		tb.Fatalf("%s contains no orders", realDataPath)
	}
	return batch
}

func TestRealDataMarshalers(t *testing.T) {
	batch := loadRealData(t)
	expected, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}

	jitEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT})
	if err != nil {
		t.Fatal(err)
	}
	defer jitEncoder.Close()

	staticEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendStatic})
	if err != nil {
		t.Fatal(err)
	}
	defer staticEncoder.Close()

	packed, err := Pack(batch)
	if err != nil {
		t.Fatal(err)
	}

	marshalers := []struct {
		name    string
		marshal func() ([]byte, error)
	}{
		{name: "reference", marshal: func() ([]byte, error) { return MarshalReference(batch) }},
		{name: "jit_with_pack", marshal: func() ([]byte, error) { return jitEncoder.Marshal(batch) }},
		{name: "jit_packed", marshal: func() ([]byte, error) { return jitEncoder.MarshalPacked(packed) }},
		{name: "static_packed", marshal: func() ([]byte, error) { return staticEncoder.MarshalPacked(packed) }},
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

func TestRealDataStatistics(t *testing.T) {
	batch := loadRealData(t)
	packed, err := Pack(batch)
	if err != nil {
		t.Fatal(err)
	}
	if !packed.stringsPlain {
		t.Fatal("real data unexpectedly contains strings requiring JSON escaping")
	}
	output, err := MarshalReference(batch)
	if err != nil {
		t.Fatal(err)
	}

	itemCount := 0
	nullTrackingCount := 0
	longStringCount := 0
	stringCount := 0
	maxStringBytes := 0

	observeString := func(value string) {
		length := len(value)
		stringCount++
		if length >= 64 {
			longStringCount++
		}
		if length > maxStringBytes {
			maxStringBytes = length
		}
	}

	for i := range batch.Orders {
		order := &batch.Orders[i]
		itemCount += len(order.Items)
		if order.Shipping.TrackingNo == nil {
			nullTrackingCount++
		}
		observeString(order.OrderID)
		observeString(order.CreatedAt)
		observeString(order.Status)
		observeString(order.Payment.Method)
		observeString(order.Buyer.Name)
		observeString(order.Shipping.City)
		observeString(order.Shipping.Address)
		if order.Shipping.TrackingNo != nil {
			observeString(*order.Shipping.TrackingNo)
		}
		observeString(order.Remark)
		for j := range order.Items {
			observeString(order.Items[j].SKU)
			observeString(order.Items[j].Title)
		}
	}

	t.Logf(
		"orders=%d items=%d source_bytes=%d compact_json_bytes=%d string_pool_bytes=%d strings=%d strings_ge_64=%d max_string_bytes=%d null_tracking=%d",
		len(batch.Orders),
		itemCount,
		fileSize(t, realDataPath),
		len(output),
		packed.StringBytes(),
		stringCount,
		longStringCount,
		maxStringBytes,
		nullTrackingCount,
	)
}

func fileSize(tb testing.TB, path string) int64 {
	tb.Helper()
	info, err := os.Stat(path)
	if err != nil {
		tb.Fatal(err)
	}
	return info.Size()
}
