package jitjson

// OrderBatch is the top-level JSON value encoded by this demo.
type OrderBatch struct {
	Orders []Order `json:"orders"`
}

type Order struct {
	OrderID   string   `json:"orderId"`
	CreatedAt string   `json:"createdAt"`
	Status    string   `json:"status"`
	Payment   Payment  `json:"payment"`
	Buyer     Buyer    `json:"buyer"`
	Shipping  Shipping `json:"shipping"`
	Items     []Item   `json:"items"`
	Remark    string   `json:"remark"`
}

type Payment struct {
	Method string `json:"method"`
	Paid   bool   `json:"paid"`
}

type Buyer struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Shipping struct {
	City       string  `json:"city"`
	Address    string  `json:"address"`
	TrackingNo *string `json:"trackingNo"`
}

type Item struct {
	SKU   string `json:"sku"`
	Title string `json:"title"`
	Qty   int64  `json:"qty"`
	Price int64  `json:"price"`
}

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
