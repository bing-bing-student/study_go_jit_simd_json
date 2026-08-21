package jitjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/bing-bing-student/study_go_jit_simd_json/internal/native"
)

func sampleBatch() OrderBatch {
	tracking := "SF-20240820"
	return OrderBatch{Orders: []Order{
		{
			OrderID:   "O0",
			CreatedAt: "2024-01-01",
			Status:    "pending",
			Payment:   Payment{Method: "ali", Paid: false},
			Buyer:     Buyer{ID: 0, Name: "张伟"},
			Shipping: Shipping{
				City:       "北京",
				Address:    "北京朝阳0号",
				TrackingNo: nil,
			},
			Items:  []Item{{SKU: "S0", Title: "JM键盘", Qty: 1, Price: math.MinInt64}},
			Remark: "工作日送",
		},
		{
			OrderID:   "O1",
			CreatedAt: "2024-01-02",
			Status:    "shipped",
			Payment:   Payment{Method: "wechat", Paid: true},
			Buyer:     Buyer{ID: math.MaxInt64, Name: "李\\\"雷"},
			Shipping: Shipping{
				City:       "上海",
				Address:    "浦东\\新区",
				TrackingNo: &tracking,
			},
			Items:  []Item{},
			Remark: "第一行\n第二行\t完成",
		},
	}}
}

func TestMarshalReferenceMatchesEncodingJSON(t *testing.T) {
	tests := []struct {
		name  string
		batch OrderBatch
	}{
		{name: "nil orders", batch: OrderBatch{}},
		{name: "empty orders", batch: OrderBatch{Orders: []Order{}}},
		{name: "sample", batch: sampleBatch()},
		{name: "nil items", batch: OrderBatch{Orders: []Order{{Items: nil}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, err := json.Marshal(test.batch)
			if err != nil {
				t.Fatalf("encoding/json: %v", err)
			}
			got, err := MarshalReference(test.batch)
			if err != nil {
				t.Fatalf("MarshalReference: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestPack(t *testing.T) {
	batch := sampleBatch()
	packed, err := Pack(batch)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if packed.OrderCount() != 2 {
		t.Fatalf("OrderCount = %d, want 2", packed.OrderCount())
	}
	if packed.ItemCount() != 1 {
		t.Fatalf("ItemCount = %d, want 1", packed.ItemCount())
	}
	if packed.StringBytes() == 0 {
		t.Fatal("StringBytes must be non-zero")
	}
	if packed.stringsPlain {
		t.Fatal("sample batch contains escaped strings")
	}
	encoded, err := MarshalReference(batch)
	if err != nil {
		t.Fatalf("MarshalReference: %v", err)
	}
	if packed.maxOutput != len(encoded) {
		t.Fatalf("maxOutput = %d, encoded length = %d", packed.maxOutput, len(encoded))
	}
	if packed.orders[0].HasTrackingNo != 0 || packed.orders[1].HasTrackingNo != 1 {
		t.Fatal("trackingNo null state was not preserved")
	}
	if packed.orders[1].ItemsNull != 0 {
		t.Fatal("empty non-nil items must not be marked null")
	}
}

func TestPackRejectsInvalidUTF8(t *testing.T) {
	batch := OrderBatch{Orders: []Order{{OrderID: string([]byte{0xff})}}}
	_, err := Pack(batch)
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("Pack error = %v, want ErrInvalidUTF8", err)
	}
}

func TestNativeLayout(t *testing.T) {
	if err := native.ValidateGoLayout(); err != nil {
		t.Fatal(err)
	}
}
