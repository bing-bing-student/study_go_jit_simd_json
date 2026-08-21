package jitjson

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"unicode/utf8"
)

func FuzzMarshal(f *testing.F) {
	f.Add("Alice", "普通备注", "SF123", true)
	f.Add("张伟", "第一行\n第二行", "", false)
	f.Add("a\\b\"c", string(make([]byte, 33)), "TRACK", true)

	encoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT})
	if err != nil {
		f.Fatalf("NewEncoder: %v", err)
	}
	defer encoder.Close()

	trustedEncoder, err := NewEncoder(Options{Mode: ModeAuto, Backend: BackendJIT, TrustUTF8: true})
	if err != nil {
		f.Fatalf("NewEncoder trusted: %v", err)
	}
	defer trustedEncoder.Close()

	f.Fuzz(func(t *testing.T, name, remark, tracking string, hasTracking bool) {
		if !utf8.ValidString(name) || !utf8.ValidString(remark) || !utf8.ValidString(tracking) {
			return
		}
		var trackingPointer *string
		if hasTracking {
			trackingCopy := tracking
			trackingPointer = &trackingCopy
		}
		batch := OrderBatch{Orders: []Order{{
			OrderID:   "fuzz",
			CreatedAt: "2024-01-01",
			Status:    "pending",
			Payment:   Payment{Method: "ali", Paid: true},
			Buyer:     Buyer{ID: -1, Name: name},
			Shipping:  Shipping{City: "北京", Address: "测试地址", TrackingNo: trackingPointer},
			Items:     []Item{{SKU: "S0", Title: name, Qty: 2, Price: 99}},
			Remark:    remark,
		}}}

		want, err := MarshalReference(batch)
		if err != nil {
			t.Fatalf("MarshalReference: %v", err)
		}
		got, err := encoder.Marshal(batch)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
		}
		trusted, err := trustedEncoder.Marshal(batch)
		if err != nil {
			t.Fatalf("trusted Marshal: %v", err)
		}
		if !bytes.Equal(trusted, want) {
			t.Fatalf("trusted output mismatch\n got: %q\nwant: %q", trusted, want)
		}
		if !json.Valid(got) {
			t.Fatalf("invalid JSON: %q", got)
		}
		var decoded OrderBatch
		if err := json.Unmarshal(got, &decoded); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if !reflect.DeepEqual(decoded, batch) {
			t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", decoded, batch)
		}
	})
}
