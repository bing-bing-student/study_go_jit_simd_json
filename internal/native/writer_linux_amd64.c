//go:build linux && amd64 && cgo

#include "internal.h"

#include <string.h>

#define LITERAL(name, value) \
    static const uint8_t name[] = value

LITERAL(k_order_id, "{\"orderId\":");
LITERAL(k_created_at, ",\"createdAt\":");
LITERAL(k_status, ",\"status\":");
LITERAL(k_payment_method, ",\"payment\":{\"method\":");
LITERAL(k_paid, ",\"paid\":");
LITERAL(k_buyer_id, "},\"buyer\":{\"id\":");
LITERAL(k_buyer_name, ",\"name\":");
LITERAL(k_shipping_city, "},\"shipping\":{\"city\":");
LITERAL(k_address, ",\"address\":");
LITERAL(k_tracking_no, ",\"trackingNo\":");
LITERAL(k_items, "},\"items\":");
LITERAL(k_remark, ",\"remark\":");
LITERAL(k_order_end, "}");
LITERAL(k_item_sku, "{\"sku\":");
LITERAL(k_item_title, ",\"title\":");
LITERAL(k_item_qty, ",\"qty\":");
LITERAL(k_item_price, ",\"price\":");
LITERAL(k_item_end, "}");
LITERAL(k_null, "null");
LITERAL(k_true, "true");
LITERAL(k_false, "false");

static int writer_reserve(jitjson_writer_t *writer, size_t additional) {
    if (writer->status != JITJSON_OK) {
        return 0;
    }
    if (additional > writer->cap - writer->len) {
        writer->status = JITJSON_ERR_NO_SPACE;
        return 0;
    }
    return 1;
}

void jitjson_writer_literal(jitjson_writer_t *writer, const uint8_t *data, size_t len) {
    if (!writer_reserve(writer, len)) {
        return;
    }
    if (len != 0) {
        memcpy(writer->data + writer->len, data, len);
        writer->len += len;
    }
}

void jitjson_writer_int64(jitjson_writer_t *writer, int64_t value) {
    uint8_t buffer[20];
    size_t length = 0;
    uint64_t magnitude = value < 0 ? 0 - (uint64_t)value : (uint64_t)value;

    do {
        buffer[length++] = (uint8_t)('0' + magnitude % 10);
        magnitude /= 10;
    } while (magnitude != 0);

    if (value < 0) {
        buffer[length++] = '-';
    }
    if (!writer_reserve(writer, length)) {
        return;
    }
    while (length != 0) {
        writer->data[writer->len++] = buffer[--length];
    }
}

void jitjson_writer_bool(jitjson_writer_t *writer, uint8_t value) {
    if (value != 0) {
        jitjson_writer_literal(writer, k_true, sizeof(k_true) - 1);
    } else {
        jitjson_writer_literal(writer, k_false, sizeof(k_false) - 1);
    }
}

static int resolve_ref(const jitjson_batch_view_t *batch, jitjson_string_ref_t ref, const uint8_t **data) {
    size_t offset = (size_t)ref.offset;
    size_t length = (size_t)ref.length;
    if (offset > batch->string_size || length > batch->string_size - offset) {
        return 0;
    }
    if (length == 0) {
        *data = batch->strings;
        return 1;
    }
    *data = batch->strings + offset;
    return 1;
}

static void write_escaped(
    jitjson_writer_t *writer,
    const jitjson_batch_view_t *batch,
    jitjson_string_ref_t ref,
    size_t (*scan)(const uint8_t *, size_t)
) {
    static const uint8_t hex[] = "0123456789abcdef";
    const uint8_t *data;
    size_t position = 0;

    if (writer->status != JITJSON_OK) {
        return;
    }
    if (!resolve_ref(batch, ref, &data)) {
        writer->status = JITJSON_ERR_INVALID_ARGUMENT;
        return;
    }

    jitjson_writer_literal(writer, (const uint8_t *)"\"", 1);
    while (position < (size_t)ref.length && writer->status == JITJSON_OK) {
        size_t remaining = (size_t)ref.length - position;
        size_t safe = scan(data + position, remaining);
        jitjson_writer_literal(writer, data + position, safe);
        position += safe;
        if (position == (size_t)ref.length) {
            break;
        }

        uint8_t value = data[position++];
        switch (value) {
            case '"': jitjson_writer_literal(writer, (const uint8_t *)"\\\"", 2); break;
            case '\\': jitjson_writer_literal(writer, (const uint8_t *)"\\\\", 2); break;
            case '\b': jitjson_writer_literal(writer, (const uint8_t *)"\\b", 2); break;
            case '\f': jitjson_writer_literal(writer, (const uint8_t *)"\\f", 2); break;
            case '\n': jitjson_writer_literal(writer, (const uint8_t *)"\\n", 2); break;
            case '\r': jitjson_writer_literal(writer, (const uint8_t *)"\\r", 2); break;
            case '\t': jitjson_writer_literal(writer, (const uint8_t *)"\\t", 2); break;
            default: {
                uint8_t escaped[6] = {'\\', 'u', '0', '0', hex[value >> 4], hex[value & 0x0f]};
                jitjson_writer_literal(writer, escaped, sizeof(escaped));
                break;
            }
        }
    }
    jitjson_writer_literal(writer, (const uint8_t *)"\"", 1);
}

void jitjson_writer_string_plain(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref) {
    const uint8_t *data;

    if (writer->status != JITJSON_OK) {
        return;
    }
    if (!resolve_ref(batch, ref, &data)) {
        writer->status = JITJSON_ERR_INVALID_ARGUMENT;
        return;
    }
    jitjson_writer_literal(writer, (const uint8_t *)"\"", 1);
    jitjson_writer_literal(writer, data, (size_t)ref.length);
    jitjson_writer_literal(writer, (const uint8_t *)"\"", 1);
}

void jitjson_writer_string_scalar(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref) {
    write_escaped(writer, batch, ref, jitjson_scan_special_scalar);
}

void jitjson_writer_string_avx2(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref) {
    if (ref.length < 64) {
        jitjson_writer_string_scalar(writer, batch, ref);
        return;
    }
    write_escaped(writer, batch, ref, jitjson_scan_special_avx2);
}

void jitjson_writer_nullable_string(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref, uint8_t present) {
    if (present == 0) {
        jitjson_writer_literal(writer, k_null, sizeof(k_null) - 1);
        return;
    }
    batch->write_string(writer, batch, ref);
}

void jitjson_writer_items(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, uint32_t start, uint32_t count, uint8_t items_null) {
    size_t first = (size_t)start;
    size_t length = (size_t)count;
    size_t i;

    if (items_null != 0) {
        if (count != 0) {
            writer->status = JITJSON_ERR_INVALID_ARGUMENT;
            return;
        }
        jitjson_writer_literal(writer, k_null, sizeof(k_null) - 1);
        return;
    }
    if (first > batch->item_count || length > batch->item_count - first) {
        writer->status = JITJSON_ERR_INVALID_ARGUMENT;
        return;
    }

    jitjson_writer_literal(writer, (const uint8_t *)"[", 1);
    for (i = 0; i < length && writer->status == JITJSON_OK; i++) {
        const jitjson_item_row_t *item = &batch->items[first + i];
        if (i != 0) {
            jitjson_writer_literal(writer, (const uint8_t *)",", 1);
        }
        jitjson_writer_literal(writer, k_item_sku, sizeof(k_item_sku) - 1);
        batch->write_string(writer, batch, item->sku);
        jitjson_writer_literal(writer, k_item_title, sizeof(k_item_title) - 1);
        batch->write_string(writer, batch, item->title);
        jitjson_writer_literal(writer, k_item_qty, sizeof(k_item_qty) - 1);
        jitjson_writer_int64(writer, item->qty);
        jitjson_writer_literal(writer, k_item_price, sizeof(k_item_price) - 1);
        jitjson_writer_int64(writer, item->price);
        jitjson_writer_literal(writer, k_item_end, sizeof(k_item_end) - 1);
    }
    jitjson_writer_literal(writer, (const uint8_t *)"]", 1);
}

jitjson_status_t jitjson_encode_order_static(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_order_id, sizeof(k_order_id) - 1);
    batch->write_string(writer, batch, order->order_id);
    jitjson_writer_literal(writer, k_created_at, sizeof(k_created_at) - 1);
    batch->write_string(writer, batch, order->created_at);
    jitjson_writer_literal(writer, k_status, sizeof(k_status) - 1);
    batch->write_string(writer, batch, order->status);
    jitjson_writer_literal(writer, k_payment_method, sizeof(k_payment_method) - 1);
    batch->write_string(writer, batch, order->payment_method);
    jitjson_writer_literal(writer, k_paid, sizeof(k_paid) - 1);
    jitjson_writer_bool(writer, order->paid);
    jitjson_writer_literal(writer, k_buyer_id, sizeof(k_buyer_id) - 1);
    jitjson_writer_int64(writer, order->buyer_id);
    jitjson_writer_literal(writer, k_buyer_name, sizeof(k_buyer_name) - 1);
    batch->write_string(writer, batch, order->buyer_name);
    jitjson_writer_literal(writer, k_shipping_city, sizeof(k_shipping_city) - 1);
    batch->write_string(writer, batch, order->city);
    jitjson_writer_literal(writer, k_address, sizeof(k_address) - 1);
    batch->write_string(writer, batch, order->address);
    jitjson_writer_literal(writer, k_tracking_no, sizeof(k_tracking_no) - 1);
    jitjson_writer_nullable_string(writer, batch, order->tracking_no, order->has_tracking_no);
    jitjson_writer_literal(writer, k_items, sizeof(k_items) - 1);
    jitjson_writer_items(writer, batch, order->item_start, order->item_count, order->items_null);
    jitjson_writer_literal(writer, k_remark, sizeof(k_remark) - 1);
    batch->write_string(writer, batch, order->remark);
    jitjson_writer_literal(writer, k_order_end, sizeof(k_order_end) - 1);
    return writer->status;
}


void jitjson_op_order_id(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_order_id, sizeof(k_order_id) - 1);
    batch->write_string(writer, batch, order->order_id);
}

void jitjson_op_created_at(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_created_at, sizeof(k_created_at) - 1);
    batch->write_string(writer, batch, order->created_at);
}

void jitjson_op_status(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_status, sizeof(k_status) - 1);
    batch->write_string(writer, batch, order->status);
}

void jitjson_op_payment(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_payment_method, sizeof(k_payment_method) - 1);
    batch->write_string(writer, batch, order->payment_method);
    jitjson_writer_literal(writer, k_paid, sizeof(k_paid) - 1);
    jitjson_writer_bool(writer, order->paid);
}

void jitjson_op_buyer(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_buyer_id, sizeof(k_buyer_id) - 1);
    jitjson_writer_int64(writer, order->buyer_id);
    jitjson_writer_literal(writer, k_buyer_name, sizeof(k_buyer_name) - 1);
    batch->write_string(writer, batch, order->buyer_name);
}

void jitjson_op_shipping(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_shipping_city, sizeof(k_shipping_city) - 1);
    batch->write_string(writer, batch, order->city);
    jitjson_writer_literal(writer, k_address, sizeof(k_address) - 1);
    batch->write_string(writer, batch, order->address);
    jitjson_writer_literal(writer, k_tracking_no, sizeof(k_tracking_no) - 1);
    jitjson_writer_nullable_string(writer, batch, order->tracking_no, order->has_tracking_no);
}

void jitjson_op_items(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_items, sizeof(k_items) - 1);
    jitjson_writer_items(writer, batch, order->item_start, order->item_count, order->items_null);
}

void jitjson_op_remark(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch) {
    jitjson_writer_literal(writer, k_remark, sizeof(k_remark) - 1);
    batch->write_string(writer, batch, order->remark);
    jitjson_writer_literal(writer, k_order_end, sizeof(k_order_end) - 1);
}
