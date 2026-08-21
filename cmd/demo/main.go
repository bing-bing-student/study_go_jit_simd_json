package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	jitjson "github.com/bing-bing-student/study_go_jit_simd_json"
)

func main() {
	orderCount := flag.Int("orders", 25000, "生成的订单数量")
	outputPath := flag.String("output", "", "可选的 JSON 输出文件")
	flag.Parse()

	batch := makeBatch(*orderCount)
	encoder, err := jitjson.NewEncoder(jitjson.Options{})
	if err != nil {
		fatal(err)
	}
	defer encoder.Close()

	packStart := time.Now()
	packed, err := jitjson.Pack(batch)
	if err != nil {
		fatal(err)
	}
	packDuration := time.Since(packStart)

	encodeStart := time.Now()
	output, err := encoder.MarshalPacked(packed)
	if err != nil {
		fatal(err)
	}
	encodeDuration := time.Since(encodeStart)

	standard, err := json.Marshal(batch)
	if err != nil {
		fatal(err)
	}
	if !bytes.Equal(output, standard) {
		fatal(fmt.Errorf("输出与 encoding/json 不一致"))
	}
	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, output, 0o644); err != nil {
			fatal(err)
		}
	}

	fmt.Printf("订单数量：      %d\n", packed.OrderCount())
	fmt.Printf("商品数量：      %d\n", packed.ItemCount())
	fmt.Printf("字符串池：      %d bytes\n", packed.StringBytes())
	fmt.Printf("JSON 大小：     %d bytes\n", len(output))
	fmt.Printf("Pack 耗时：     %s\n", packDuration)
	fmt.Printf("JIT 编码耗时：  %s\n", encodeDuration)
	fmt.Printf("JIT 代码大小：  %d bytes\n", encoder.CodeSize())
	fmt.Printf("CPU 支持 AVX2： %t\n", jitjson.SupportsAVX2())
	fmt.Println("结果验证：      通过")
}

func makeBatch(count int) jitjson.OrderBatch {
	orders := make([]jitjson.Order, count)
	statuses := []string{"pending", "paid", "shipped", "completed"}
	methods := []string{"ali", "wechat", "card"}
	cities := []string{"北京", "上海", "深圳", "杭州"}
	names := []string{"张伟", "李娜", "王强", "刘洋"}

	for i := range orders {
		var tracking *string
		if i%3 != 0 {
			value := fmt.Sprintf("SF%012d", i)
			tracking = &value
		}
		itemCount := 1 + i%3
		items := make([]jitjson.Item, itemCount)
		for itemIndex := range items {
			items[itemIndex] = jitjson.Item{
				SKU:   fmt.Sprintf("S%d-%d", i, itemIndex),
				Title: []string{"机械键盘", "无线鼠标", "显示器支架"}[itemIndex%3],
				Qty:   int64(1 + itemIndex),
				Price: int64(9900 + itemIndex*5000),
			}
		}
		remark := "工作日送"
		if i%10 == 0 {
			remark = strings.Repeat("请送到前台，", 16) + "到达后电话联系\n谢谢"
		}
		orders[i] = jitjson.Order{
			OrderID:   fmt.Sprintf("O%d", i),
			CreatedAt: "2024-01-01",
			Status:    statuses[i%len(statuses)],
			Payment:   jitjson.Payment{Method: methods[i%len(methods)], Paid: i%4 != 0},
			Buyer:     jitjson.Buyer{ID: int64(i), Name: names[i%len(names)]},
			Shipping: jitjson.Shipping{
				City:       cities[i%len(cities)],
				Address:    fmt.Sprintf("测试路%d号", i),
				TrackingNo: tracking,
			},
			Items:  items,
			Remark: remark,
		}
	}
	return jitjson.OrderBatch{Orders: orders}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误：", err)
	os.Exit(1)
}
