package jitjson

import (
	"strconv"
	"unicode/utf8"
)

func MarshalReference(batch OrderBatch) ([]byte, error) {
	capacity, err := estimateMaxOutput(batch)
	if err != nil {
		return nil, err
	}
	output := make([]byte, 0, capacity)
	output = append(output, batchPrefix...)
	if batch.Orders == nil {
		output = append(output, "null"...)
	} else {
		output = append(output, '[')
		for i := range batch.Orders {
			if i > 0 {
				output = append(output, ',')
			}
			if output, err = appendReferenceOrder(output, &batch.Orders[i]); err != nil {
				return nil, err
			}
		}
		output = append(output, ']')
	}
	output = append(output, batchSuffix...)
	return output, nil
}

func appendReferenceOrder(output []byte, order *Order) ([]byte, error) {
	var err error
	output = append(output, orderIDPrefix...)
	if output, err = appendJSONString(output, order.OrderID); err != nil {
		return nil, err
	}
	output = append(output, createdAtPrefix...)
	if output, err = appendJSONString(output, order.CreatedAt); err != nil {
		return nil, err
	}
	output = append(output, statusPrefix...)
	if output, err = appendJSONString(output, order.Status); err != nil {
		return nil, err
	}
	output = append(output, paymentMethodPrefix...)
	if output, err = appendJSONString(output, order.Payment.Method); err != nil {
		return nil, err
	}
	output = append(output, paidPrefix...)
	output = strconv.AppendBool(output, order.Payment.Paid)
	output = append(output, paymentSuffix...)

	output = append(output, buyerIDPrefix...)
	output = strconv.AppendInt(output, order.Buyer.ID, 10)
	output = append(output, buyerNamePrefix...)
	if output, err = appendJSONString(output, order.Buyer.Name); err != nil {
		return nil, err
	}
	output = append(output, buyerSuffix...)

	output = append(output, shippingCityPrefix...)
	if output, err = appendJSONString(output, order.Shipping.City); err != nil {
		return nil, err
	}
	output = append(output, addressPrefix...)
	if output, err = appendJSONString(output, order.Shipping.Address); err != nil {
		return nil, err
	}
	output = append(output, trackingNoPrefix...)
	if order.Shipping.TrackingNo == nil {
		output = append(output, "null"...)
	} else if output, err = appendJSONString(output, *order.Shipping.TrackingNo); err != nil {
		return nil, err
	}
	output = append(output, shippingSuffix...)

	output = append(output, itemsPrefix...)
	if order.Items == nil {
		output = append(output, "null"...)
	} else {
		output = append(output, '[')
		for i := range order.Items {
			if i > 0 {
				output = append(output, ',')
			}
			if output, err = appendReferenceItem(output, &order.Items[i]); err != nil {
				return nil, err
			}
		}
		output = append(output, ']')
	}

	output = append(output, remarkPrefix...)
	if output, err = appendJSONString(output, order.Remark); err != nil {
		return nil, err
	}
	output = append(output, orderSuffix...)
	return output, nil
}

func appendReferenceItem(output []byte, item *Item) ([]byte, error) {
	var err error
	output = append(output, itemSKUPrefix...)
	if output, err = appendJSONString(output, item.SKU); err != nil {
		return nil, err
	}
	output = append(output, itemTitlePrefix...)
	if output, err = appendJSONString(output, item.Title); err != nil {
		return nil, err
	}
	output = append(output, itemQtyPrefix...)
	output = strconv.AppendInt(output, item.Qty, 10)
	output = append(output, itemPricePrefix...)
	output = strconv.AppendInt(output, item.Price, 10)
	output = append(output, itemSuffix...)
	return output, nil
}

func appendJSONString(output []byte, value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, ErrInvalidUTF8
	}
	const hex = "0123456789abcdef"
	output = append(output, '"')
	for i := 0; i < len(value); i++ {
		char := value[i]
		if char >= 0x20 && char != '"' && char != '\\' {
			output = append(output, char)
			continue
		}
		switch char {
		case '"', '\\':
			output = append(output, '\\', char)
		case '\b':
			output = append(output, '\\', 'b')
		case '\f':
			output = append(output, '\\', 'f')
		case '\n':
			output = append(output, '\\', 'n')
		case '\r':
			output = append(output, '\\', 'r')
		case '\t':
			output = append(output, '\\', 't')
		default:
			output = append(output, '\\', 'u', '0', '0', hex[char>>4], hex[char&0x0f])
		}
	}
	output = append(output, '"')
	return output, nil
}
