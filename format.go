package jitjson

const (
	batchPrefix         = `{"orders":`
	orderIDPrefix       = `{"orderId":`
	createdAtPrefix     = `,"createdAt":`
	statusPrefix        = `,"status":`
	paymentMethodPrefix = `,"payment":{"method":`
	paidPrefix          = `,"paid":`
	paymentSuffix       = `}`
	buyerIDPrefix       = `,"buyer":{"id":`
	buyerNamePrefix     = `,"name":`
	buyerSuffix         = `}`
	shippingCityPrefix  = `,"shipping":{"city":`
	addressPrefix       = `,"address":`
	trackingNoPrefix    = `,"trackingNo":`
	shippingSuffix      = `}`
	itemsPrefix         = `,"items":`
	remarkPrefix        = `,"remark":`
	orderSuffix         = `}`
	batchSuffix         = `}`

	itemSKUPrefix   = `{"sku":`
	itemTitlePrefix = `,"title":`
	itemQtyPrefix   = `,"qty":`
	itemPricePrefix = `,"price":`
	itemSuffix      = `}`
)
