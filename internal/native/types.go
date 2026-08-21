package native

import (
	"fmt"
	"unsafe"
)

type StringRef struct {
	Offset uint32
	Length uint32
}

type OrderRow struct {
	OrderID       StringRef
	CreatedAt     StringRef
	Status        StringRef
	PaymentMethod StringRef
	BuyerName     StringRef
	City          StringRef
	Address       StringRef
	TrackingNo    StringRef
	Remark        StringRef
	ItemStart     uint32
	ItemCount     uint32
	BuyerID       int64
	Paid          uint8
	HasTrackingNo uint8
	ItemsNull     uint8
	_             [5]byte
}

type ItemRow struct {
	SKU   StringRef
	Title StringRef
	Qty   int64
	Price int64
}

const (
	OrderRowSize = 96
	ItemRowSize  = 32

	StatusOK              = 0
	StatusInvalidArgument = 1
	StatusNoSpace         = 2
	StatusUnsupportedCPU  = 3
	StatusJITAlloc        = 4
	StatusJITProtect      = 5
	StatusInternal        = 6
)

func ValidateGoLayout() error {
	if unsafe.Sizeof(OrderRow{}) != OrderRowSize {
		return fmt.Errorf("native: OrderRow size is %d, want %d", unsafe.Sizeof(OrderRow{}), OrderRowSize)
	}
	if unsafe.Offsetof(OrderRow{}.ItemStart) != 72 ||
		unsafe.Offsetof(OrderRow{}.BuyerID) != 80 ||
		unsafe.Offsetof(OrderRow{}.Paid) != 88 ||
		unsafe.Offsetof(OrderRow{}.HasTrackingNo) != 89 ||
		unsafe.Offsetof(OrderRow{}.ItemsNull) != 90 {
		return fmt.Errorf("native: unexpected OrderRow field layout")
	}
	if unsafe.Sizeof(ItemRow{}) != ItemRowSize {
		return fmt.Errorf("native: ItemRow size is %d, want %d", unsafe.Sizeof(ItemRow{}), ItemRowSize)
	}
	if unsafe.Offsetof(ItemRow{}.Qty) != 16 || unsafe.Offsetof(ItemRow{}.Price) != 24 {
		return fmt.Errorf("native: unexpected ItemRow field layout")
	}
	return nil
}
