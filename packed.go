package jitjson

import (
	"math"
	"unicode/utf8"

	"github.com/bing-bing-student/study_go_jit_simd_json/internal/native"
)

type PackedBatch struct {
	orders       []native.OrderRow
	items        []native.ItemRow
	strings      []byte
	ordersNull   bool
	stringsPlain bool
	validateUTF8 bool
	maxOutput    int
}

func (p *PackedBatch) OrderCount() int {
	if p == nil {
		return 0
	}
	return len(p.orders)
}

func (p *PackedBatch) ItemCount() int {
	if p == nil {
		return 0
	}
	return len(p.items)
}

func (p *PackedBatch) StringBytes() int {
	if p == nil {
		return 0
	}
	return len(p.strings)
}

func Pack(batch OrderBatch) (*PackedBatch, error) {
	packed := &PackedBatch{}
	if err := packInto(batch, packed); err != nil {
		return nil, err
	}
	return packed, nil
}

func packInto(batch OrderBatch, packed *PackedBatch) error {
	return packIntoMode(batch, packed, true, true)
}

func packIntoReusable(batch OrderBatch, packed *PackedBatch, validateUTF8 bool) error {
	return packIntoMode(batch, packed, false, validateUTF8)
}

func packIntoMode(batch OrderBatch, packed *PackedBatch, preallocate bool, validateUTF8 bool) error {
	if err := native.ValidateGoLayout(); err != nil {
		return err
	}

	if preallocate {
		itemCount, stringBytes, capacityErr := packedCapacities(batch)
		if capacityErr != nil {
			return capacityErr
		}
		packed.reset(len(batch.Orders), itemCount, stringBytes)
	} else {
		packed.reset(len(batch.Orders), 0, 0)
	}
	packed.ordersNull = batch.Orders == nil
	packed.validateUTF8 = validateUTF8

	var outputSize sizeAccumulator
	if err := outputSize.addLiteral(batchPrefix); err != nil {
		return err
	}
	if batch.Orders == nil {
		if err := outputSize.add(4); err != nil {
			return err
		}
	} else {
		if err := outputSize.add(2); err != nil {
			return err
		}
		for i := range batch.Orders {
			if i > 0 {
				if err := outputSize.add(1); err != nil {
					return err
				}
			}
			if err := packOrder(packed, &outputSize, &batch.Orders[i], &packed.orders[i]); err != nil {
				return err
			}
		}
	}
	if err := outputSize.addLiteral(batchSuffix); err != nil {
		return err
	}
	packed.maxOutput = int(outputSize.value)
	return nil
}

func (p *PackedBatch) reset(orderCount int, itemCount int, stringBytes int) {
	if cap(p.orders) < orderCount {
		p.orders = make([]native.OrderRow, orderCount)
	} else {
		p.orders = p.orders[:orderCount]
	}
	if cap(p.items) < itemCount {
		p.items = make([]native.ItemRow, 0, itemCount)
	} else {
		p.items = p.items[:0]
	}
	if cap(p.strings) < stringBytes {
		p.strings = make([]byte, 0, stringBytes)
	} else {
		p.strings = p.strings[:0]
	}
	p.stringsPlain = true
	p.maxOutput = 0
}

func (p *PackedBatch) retainedBytes() int {
	return cap(p.orders)*native.OrderRowSize + cap(p.items)*native.ItemRowSize + cap(p.strings)
}

func packOrder(packed *PackedBatch, outputSize *sizeAccumulator, order *Order, row *native.OrderRow) error {
	row.TrackingNo = native.StringRef{}
	row.Paid = 0
	row.HasTrackingNo = 0
	row.ItemsNull = 0

	var fieldErr error
	if row.OrderID, fieldErr = appendPackedField(packed, outputSize, orderIDPrefix, order.OrderID); fieldErr != nil {
		return fieldErr
	}
	if row.CreatedAt, fieldErr = appendPackedField(packed, outputSize, createdAtPrefix, order.CreatedAt); fieldErr != nil {
		return fieldErr
	}
	if row.Status, fieldErr = appendPackedField(packed, outputSize, statusPrefix, order.Status); fieldErr != nil {
		return fieldErr
	}
	if row.PaymentMethod, fieldErr = appendPackedField(packed, outputSize, paymentMethodPrefix, order.Payment.Method); fieldErr != nil {
		return fieldErr
	}
	if err := outputSize.addLiteral(paidPrefix); err != nil {
		return err
	}
	if err := outputSize.addBool(order.Payment.Paid); err != nil {
		return err
	}
	if err := outputSize.addLiteral(paymentSuffix + buyerIDPrefix); err != nil {
		return err
	}
	if err := outputSize.addInt64(order.Buyer.ID); err != nil {
		return err
	}
	if row.BuyerName, fieldErr = appendPackedField(packed, outputSize, buyerNamePrefix, order.Buyer.Name); fieldErr != nil {
		return fieldErr
	}
	if row.City, fieldErr = appendPackedField(packed, outputSize, buyerSuffix+shippingCityPrefix, order.Shipping.City); fieldErr != nil {
		return fieldErr
	}
	if row.Address, fieldErr = appendPackedField(packed, outputSize, addressPrefix, order.Shipping.Address); fieldErr != nil {
		return fieldErr
	}
	if err := outputSize.addLiteral(trackingNoPrefix); err != nil {
		return err
	}
	if order.Shipping.TrackingNo == nil {
		if err := outputSize.add(4); err != nil {
			return err
		}
	} else {
		row.HasTrackingNo = 1
		if row.TrackingNo, fieldErr = appendPackedString(packed, outputSize, *order.Shipping.TrackingNo); fieldErr != nil {
			return fieldErr
		}
	}
	if err := outputSize.addLiteral(shippingSuffix + itemsPrefix); err != nil {
		return err
	}

	if len(packed.items) > math.MaxUint32 || len(order.Items) > math.MaxUint32-len(packed.items) {
		return ErrPackedTooLarge
	}
	row.ItemStart = uint32(len(packed.items))
	row.ItemCount = uint32(len(order.Items))
	row.BuyerID = order.Buyer.ID
	if order.Payment.Paid {
		row.Paid = 1
	}
	if order.Items == nil {
		row.ItemsNull = 1
		if err := outputSize.add(4); err != nil {
			return err
		}
	} else {
		if err := outputSize.add(2); err != nil {
			return err
		}
		for i := range order.Items {
			if i > 0 {
				if err := outputSize.add(1); err != nil {
					return err
				}
			}
			if err := packItem(packed, outputSize, &order.Items[i]); err != nil {
				return err
			}
		}
	}

	if row.Remark, fieldErr = appendPackedField(packed, outputSize, remarkPrefix, order.Remark); fieldErr != nil {
		return fieldErr
	}
	return outputSize.addLiteral(orderSuffix)
}

func packItem(packed *PackedBatch, outputSize *sizeAccumulator, item *Item) error {
	var row native.ItemRow
	var fieldErr error
	if row.SKU, fieldErr = appendPackedField(packed, outputSize, itemSKUPrefix, item.SKU); fieldErr != nil {
		return fieldErr
	}
	if row.Title, fieldErr = appendPackedField(packed, outputSize, itemTitlePrefix, item.Title); fieldErr != nil {
		return fieldErr
	}
	if err := outputSize.addLiteral(itemQtyPrefix); err != nil {
		return err
	}
	if err := outputSize.addInt64(item.Qty); err != nil {
		return err
	}
	if err := outputSize.addLiteral(itemPricePrefix); err != nil {
		return err
	}
	if err := outputSize.addInt64(item.Price); err != nil {
		return err
	}
	if err := outputSize.addLiteral(itemSuffix); err != nil {
		return err
	}

	row.Qty = item.Qty
	row.Price = item.Price
	packed.items = append(packed.items, row)
	return nil
}

func packedCapacities(batch OrderBatch) (int, int, error) {
	var itemCount uint64
	var stringBytes uint64
	addString := func(value string) error {
		stringBytes += uint64(len(value))
		if stringBytes > math.MaxUint32 {
			return ErrPackedTooLarge
		}
		return nil
	}

	for i := range batch.Orders {
		order := &batch.Orders[i]
		itemCount += uint64(len(order.Items))
		if itemCount > math.MaxUint32 {
			return 0, 0, ErrPackedTooLarge
		}

		values := [...]string{
			order.OrderID,
			order.CreatedAt,
			order.Status,
			order.Payment.Method,
			order.Buyer.Name,
			order.Shipping.City,
			order.Shipping.Address,
			order.Remark,
		}
		for _, value := range values {
			if err := addString(value); err != nil {
				return 0, 0, err
			}
		}
		if order.Shipping.TrackingNo != nil {
			if err := addString(*order.Shipping.TrackingNo); err != nil {
				return 0, 0, err
			}
		}
		for j := range order.Items {
			if err := addString(order.Items[j].SKU); err != nil {
				return 0, 0, err
			}
			if err := addString(order.Items[j].Title); err != nil {
				return 0, 0, err
			}
		}
	}

	const maxInt = uint64(^uint(0) >> 1)
	if itemCount > maxInt || stringBytes > maxInt {
		return 0, 0, ErrPackedTooLarge
	}
	return int(itemCount), int(stringBytes), nil
}

func appendPackedField(packed *PackedBatch, outputSize *sizeAccumulator, literal string, value string) (native.StringRef, error) {
	if err := outputSize.addLiteral(literal); err != nil {
		return native.StringRef{}, err
	}
	return appendPackedString(packed, outputSize, value)
}

func appendPackedString(packed *PackedBatch, outputSize *sizeAccumulator, value string) (native.StringRef, error) {
	if len(packed.strings) > math.MaxUint32 || len(value) > math.MaxUint32-len(packed.strings) {
		return native.StringRef{}, ErrPackedTooLarge
	}
	stringSize, plain, err := analyzeJSONString(value, packed.validateUTF8)
	if err != nil {
		return native.StringRef{}, err
	}
	if err := outputSize.add(stringSize); err != nil {
		return native.StringRef{}, err
	}
	if !plain {
		packed.stringsPlain = false
	}
	ref := native.StringRef{Offset: uint32(len(packed.strings)), Length: uint32(len(value))}
	packed.strings = append(packed.strings, value...)
	return ref, nil
}

type sizeAccumulator struct {
	value uint64
}

func (s *sizeAccumulator) add(value uint64) error {
	const maxInt = uint64(^uint(0) >> 1)
	if value > maxInt-s.value {
		return ErrOutputTooLarge
	}
	s.value += value
	return nil
}

func (s *sizeAccumulator) addLiteral(value string) error {
	return s.add(uint64(len(value)))
}

func (s *sizeAccumulator) addBool(value bool) error {
	if value {
		return s.add(4)
	}
	return s.add(5)
}

func (s *sizeAccumulator) addInt64(value int64) error {
	magnitude := uint64(value)
	length := uint64(1)
	if value < 0 {
		length++
		magnitude = 0 - magnitude
	}
	for magnitude >= 10 {
		length++
		magnitude /= 10
	}
	return s.add(length)
}

func (s *sizeAccumulator) addString(value string) error {
	stringSize, err := encodedJSONStringSize(value)
	if err != nil {
		return err
	}
	return s.add(stringSize)
}

func encodedJSONStringSize(value string) (uint64, error) {
	size, _, err := analyzeJSONString(value, true)
	return size, err
}

func analyzeJSONString(value string, validateUTF8 bool) (uint64, bool, error) {
	if validateUTF8 && !utf8.ValidString(value) {
		return 0, false, ErrInvalidUTF8
	}

	size := uint64(len(value)) + 2
	plain := true
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			size++
			plain = false
		case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x0b, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
			0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c,
			0x1d, 0x1e, 0x1f:
			size += 5
			plain = false
		}
	}
	return size, plain, nil
}

func estimateMaxOutput(batch OrderBatch) (int, error) {
	var size sizeAccumulator
	if err := size.addLiteral(batchPrefix); err != nil {
		return 0, err
	}
	if batch.Orders == nil {
		if err := size.add(4); err != nil {
			return 0, err
		}
	} else {
		if err := size.add(2); err != nil {
			return 0, err
		}
		for i := range batch.Orders {
			if i > 0 {
				if err := size.add(1); err != nil {
					return 0, err
				}
			}
			if err := estimateOrderMax(&size, &batch.Orders[i]); err != nil {
				return 0, err
			}
		}
	}
	if err := size.addLiteral(batchSuffix); err != nil {
		return 0, err
	}
	return int(size.value), nil
}

func estimateOrderMax(size *sizeAccumulator, order *Order) error {
	literals := []string{orderIDPrefix, createdAtPrefix, statusPrefix, paymentMethodPrefix}
	strings := []string{order.OrderID, order.CreatedAt, order.Status, order.Payment.Method}
	for i := range literals {
		if err := size.addLiteral(literals[i]); err != nil {
			return err
		}
		if err := size.addString(strings[i]); err != nil {
			return err
		}
	}
	if err := size.addLiteral(paidPrefix); err != nil {
		return err
	}
	if err := size.addBool(order.Payment.Paid); err != nil {
		return err
	}
	if err := size.addLiteral(paymentSuffix + buyerIDPrefix); err != nil {
		return err
	}
	if err := size.addInt64(order.Buyer.ID); err != nil {
		return err
	}
	if err := size.addLiteral(buyerNamePrefix); err != nil {
		return err
	}
	if err := size.addString(order.Buyer.Name); err != nil {
		return err
	}
	if err := size.addLiteral(buyerSuffix + shippingCityPrefix); err != nil {
		return err
	}
	if err := size.addString(order.Shipping.City); err != nil {
		return err
	}
	if err := size.addLiteral(addressPrefix); err != nil {
		return err
	}
	if err := size.addString(order.Shipping.Address); err != nil {
		return err
	}
	if err := size.addLiteral(trackingNoPrefix); err != nil {
		return err
	}
	if order.Shipping.TrackingNo == nil {
		if err := size.add(4); err != nil {
			return err
		}
	} else if err := size.addString(*order.Shipping.TrackingNo); err != nil {
		return err
	}
	if err := size.addLiteral(shippingSuffix + itemsPrefix); err != nil {
		return err
	}
	if order.Items == nil {
		if err := size.add(4); err != nil {
			return err
		}
	} else {
		if err := size.add(2); err != nil {
			return err
		}
		for i := range order.Items {
			if i > 0 {
				if err := size.add(1); err != nil {
					return err
				}
			}
			if err := estimateItemMax(size, &order.Items[i]); err != nil {
				return err
			}
		}
	}
	if err := size.addLiteral(remarkPrefix); err != nil {
		return err
	}
	if err := size.addString(order.Remark); err != nil {
		return err
	}
	return size.addLiteral(orderSuffix)
}

func estimateItemMax(size *sizeAccumulator, item *Item) error {
	if err := size.addLiteral(itemSKUPrefix); err != nil {
		return err
	}
	if err := size.addString(item.SKU); err != nil {
		return err
	}
	if err := size.addLiteral(itemTitlePrefix); err != nil {
		return err
	}
	if err := size.addString(item.Title); err != nil {
		return err
	}
	if err := size.addLiteral(itemQtyPrefix); err != nil {
		return err
	}
	if err := size.addInt64(item.Qty); err != nil {
		return err
	}
	if err := size.addLiteral(itemPricePrefix); err != nil {
		return err
	}
	if err := size.addInt64(item.Price); err != nil {
		return err
	}
	return size.addLiteral(itemSuffix)
}
