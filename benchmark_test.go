package jitjson

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

var benchmarkOutput []byte
var benchmarkPacked *PackedBatch

func BenchmarkMarshal(b *testing.B) {
	batch := makeBenchmarkBatch(100)
	packed, err := Pack(batch)
	if err != nil {
		b.Fatal(err)
	}
	reference, err := MarshalReference(batch)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(reference)))

	b.Run("encoding_json", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			output, err := json.Marshal(batch)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkOutput = output
		}
	})

	b.Run("reference", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			output, err := MarshalReference(batch)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkOutput = output
		}
	})

	b.Run("pack", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			value, err := Pack(batch)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkPacked = value
		}
	})

	benchEncoder := func(name string, options Options, includePack bool) {
		b.Run(name, func(b *testing.B) {
			encoder, err := NewEncoder(options)
			if err != nil {
				b.Fatal(err)
			}
			defer encoder.Close()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var output []byte
				if includePack {
					output, err = encoder.Marshal(batch)
				} else {
					output, err = encoder.MarshalPacked(packed)
				}
				if err != nil {
					b.Fatal(err)
				}
				benchmarkOutput = output
			}
		})
	}

	benchEncoder("static_scalar", Options{Mode: ModeScalar, Backend: BackendStatic}, false)
	benchEncoder("jit_scalar", Options{Mode: ModeScalar, Backend: BackendJIT}, false)
	if SupportsAVX2() {
		benchEncoder("static_avx2", Options{Mode: ModeAVX2, Backend: BackendStatic}, false)
		benchEncoder("jit_avx2", Options{Mode: ModeAVX2, Backend: BackendJIT}, false)
		benchEncoder("jit_avx2_with_pack", Options{Mode: ModeAVX2, Backend: BackendJIT}, true)
	}
}

func makeBenchmarkBatch(count int) OrderBatch {
	orders := make([]Order, count)
	for i := range orders {
		var tracking *string
		if i%3 != 0 {
			value := fmt.Sprintf("SF%012d", i)
			tracking = &value
		}
		remark := "工作日送"
		if i%10 == 0 {
			remark = strings.Repeat("long-ascii-remark-", 16) + "\n需要电话确认"
		}
		orders[i] = Order{
			OrderID:   fmt.Sprintf("O%d", i),
			CreatedAt: "2024-01-01",
			Status:    []string{"pending", "paid", "shipped", "completed"}[i%4],
			Payment:   Payment{Method: []string{"ali", "wechat", "card"}[i%3], Paid: i%4 != 0},
			Buyer:     Buyer{ID: int64(i), Name: []string{"张伟", "李娜", "王强"}[i%3]},
			Shipping: Shipping{
				City:       []string{"北京", "上海", "深圳"}[i%3],
				Address:    fmt.Sprintf("测试地址%d号", i),
				TrackingNo: tracking,
			},
			Items: []Item{
				{SKU: fmt.Sprintf("S%d", i), Title: "机械键盘", Qty: 1, Price: 19900},
				{SKU: fmt.Sprintf("S%d-B", i), Title: "鼠标", Qty: 2, Price: 9900},
			},
			Remark: remark,
		}
	}
	return OrderBatch{Orders: orders}
}

func BenchmarkLongStringSIMD(b *testing.B) {
	for _, length := range []int{16, 32, 256, 4096} {
		b.Run(fmt.Sprintf("bytes_%d", length), func(b *testing.B) {
			batch := OrderBatch{Orders: []Order{{
				OrderID:   "O1",
				CreatedAt: "2024-01-01",
				Status:    "paid",
				Payment:   Payment{Method: "ali", Paid: true},
				Buyer:     Buyer{ID: 1, Name: "张伟"},
				Shipping:  Shipping{City: "北京", Address: "测试地址"},
				Items:     []Item{},
				Remark:    strings.Repeat("a", length-1) + "\n",
			}}}
			packed, err := Pack(batch)
			if err != nil {
				b.Fatal(err)
			}
			reference, err := MarshalReference(batch)
			if err != nil {
				b.Fatal(err)
			}

			benchMode := func(name string, mode Mode) {
				b.Run(name, func(b *testing.B) {
					encoder, err := NewEncoder(Options{Mode: mode, Backend: BackendJIT})
					if err != nil {
						b.Fatal(err)
					}
					defer encoder.Close()
					b.SetBytes(int64(len(reference)))
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						output, marshalErr := encoder.MarshalPacked(packed)
						if marshalErr != nil {
							b.Fatal(marshalErr)
						}
						benchmarkOutput = output
					}
				})
			}

			benchMode("scalar", ModeScalar)
			if SupportsAVX2() {
				benchMode("avx2", ModeAVX2)
			}
		})
	}
}
