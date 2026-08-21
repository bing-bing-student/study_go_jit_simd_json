{
  orders: [
    .orders[]
    | {
        orderId,
        createdAt,
        status,
        payment: {
          method: .payment.method,
          paid: .payment.paid
        },
        buyer: {
          id: .buyer.id,
          name: .buyer.name
        },
        shipping: {
          city: .shipping.city,
          address: .shipping.address,
          trackingNo: .shipping.trackingNo
        },
        items: [
          .items[]
          | {
              sku,
              title,
              qty,
              price
            }
        ],
        remark
      }
  ]
}
