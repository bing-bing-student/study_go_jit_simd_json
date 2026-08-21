#include "jitjson.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static jitjson_string_ref_t add_string(uint8_t *pool, size_t *pool_len, const char *value) {
    size_t length = strlen(value);
    jitjson_string_ref_t ref;
    ref.offset = (uint32_t)*pool_len;
    ref.length = (uint32_t)length;
    memcpy(pool + *pool_len, value, length);
    *pool_len += length;
    return ref;
}

static int run_mode(jitjson_mode_t mode, jitjson_backend_t backend) {
    static const char expected[] =
        "{\"orders\":[{\"orderId\":\"O1\",\"createdAt\":\"2024-01-01\","
        "\"status\":\"paid\",\"payment\":{\"method\":\"ali\",\"paid\":true},"
        "\"buyer\":{\"id\":7,\"name\":\"Alice\"},\"shipping\":{\"city\":\"Beijing\","
        "\"address\":\"Road 1\",\"trackingNo\":null},\"items\":[{\"sku\":\"S1\","
        "\"title\":\"Keyboard\",\"qty\":2,\"price\":199}],\"remark\":\"line\\nnext\"}]}";
    uint8_t pool[1024];
    size_t pool_len = 0;
    uint8_t output[4096];
    size_t written = 0;
    jitjson_encoder_t *encoder = NULL;
    jitjson_order_row_t order;
    jitjson_item_row_t item;
    jitjson_status_t status;

    memset(&order, 0, sizeof(order));
    memset(&item, 0, sizeof(item));
    order.order_id = add_string(pool, &pool_len, "O1");
    order.created_at = add_string(pool, &pool_len, "2024-01-01");
    order.status = add_string(pool, &pool_len, "paid");
    order.payment_method = add_string(pool, &pool_len, "ali");
    order.buyer_name = add_string(pool, &pool_len, "Alice");
    order.city = add_string(pool, &pool_len, "Beijing");
    order.address = add_string(pool, &pool_len, "Road 1");
    order.remark = add_string(pool, &pool_len, "line\nnext");
    order.item_start = 0;
    order.item_count = 1;
    order.buyer_id = 7;
    order.paid = 1;

    item.sku = add_string(pool, &pool_len, "S1");
    item.title = add_string(pool, &pool_len, "Keyboard");
    item.qty = 2;
    item.price = 199;

    status = jitjson_encoder_create(mode, backend, &encoder);
    if (status != JITJSON_OK) {
        fprintf(stderr, "create failed: %s\n", jitjson_status_message(status));
        return 1;
    }
    status = jitjson_encoder_encode(
        encoder,
        &order,
        1,
        0,
        &item,
        1,
        pool,
        pool_len,
        0,
        output,
        sizeof(output),
        &written
    );
    if (status != JITJSON_OK) {
        fprintf(stderr, "encode failed: %s\n", jitjson_status_message(status));
        jitjson_encoder_destroy(encoder);
        return 1;
    }
    if (written != sizeof(expected) - 1 || memcmp(output, expected, written) != 0) {
        fprintf(stderr, "output mismatch\n got: %.*s\nwant: %s\n", (int)written, output, expected);
        jitjson_encoder_destroy(encoder);
        return 1;
    }
    jitjson_encoder_destroy(encoder);
    return 0;
}

int main(void) {
    if (run_mode(JITJSON_MODE_SCALAR, JITJSON_BACKEND_STATIC) != 0) {
        return 1;
    }
    if (run_mode(JITJSON_MODE_SCALAR, JITJSON_BACKEND_JIT) != 0) {
        return 1;
    }
    if (jitjson_cpu_supports_avx2()) {
        if (run_mode(JITJSON_MODE_AVX2, JITJSON_BACKEND_STATIC) != 0) {
            return 1;
        }
        if (run_mode(JITJSON_MODE_AVX2, JITJSON_BACKEND_JIT) != 0) {
            return 1;
        }
    }
    puts("C sanitizer test passed");
    return 0;
}
