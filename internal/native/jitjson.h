#ifndef JITJSON_H
#define JITJSON_H

#include <stddef.h>
#include <stdint.h>

typedef enum {
    JITJSON_OK = 0,
    JITJSON_ERR_INVALID_ARGUMENT = 1,
    JITJSON_ERR_NO_SPACE = 2,
    JITJSON_ERR_UNSUPPORTED_CPU = 3,
    JITJSON_ERR_JIT_ALLOC = 4,
    JITJSON_ERR_JIT_PROTECT = 5,
    JITJSON_ERR_INTERNAL = 6
} jitjson_status_t;

typedef enum {
    JITJSON_MODE_AUTO = 0,
    JITJSON_MODE_SCALAR = 1,
    JITJSON_MODE_AVX2 = 2
} jitjson_mode_t;

typedef enum {
    JITJSON_BACKEND_JIT = 0,
    JITJSON_BACKEND_STATIC = 1
} jitjson_backend_t;

typedef struct {
    uint32_t offset;
    uint32_t length;
} jitjson_string_ref_t;

typedef struct {
    jitjson_string_ref_t order_id;
    jitjson_string_ref_t created_at;
    jitjson_string_ref_t status;
    jitjson_string_ref_t payment_method;
    jitjson_string_ref_t buyer_name;
    jitjson_string_ref_t city;
    jitjson_string_ref_t address;
    jitjson_string_ref_t tracking_no;
    jitjson_string_ref_t remark;
    uint32_t item_start;
    uint32_t item_count;
    int64_t buyer_id;
    uint8_t paid;
    uint8_t has_tracking_no;
    uint8_t items_null;
    uint8_t padding[5];
} jitjson_order_row_t;

typedef struct {
    jitjson_string_ref_t sku;
    jitjson_string_ref_t title;
    int64_t qty;
    int64_t price;
} jitjson_item_row_t;

_Static_assert(sizeof(jitjson_string_ref_t) == 8, "unexpected string ref size");
_Static_assert(sizeof(jitjson_order_row_t) == 96, "unexpected order row size");
_Static_assert(offsetof(jitjson_order_row_t, item_start) == 72, "unexpected item_start offset");
_Static_assert(offsetof(jitjson_order_row_t, buyer_id) == 80, "unexpected buyer_id offset");
_Static_assert(offsetof(jitjson_order_row_t, paid) == 88, "unexpected paid offset");
_Static_assert(offsetof(jitjson_order_row_t, items_null) == 90, "unexpected items_null offset");
_Static_assert(sizeof(jitjson_item_row_t) == 32, "unexpected item row size");

typedef struct jitjson_encoder jitjson_encoder_t;

jitjson_status_t jitjson_encoder_create(
    jitjson_mode_t mode,
    jitjson_backend_t backend,
    jitjson_encoder_t **out_encoder
);

jitjson_status_t jitjson_encoder_encode(
    const jitjson_encoder_t *encoder,
    const jitjson_order_row_t *orders,
    size_t order_count,
    uint8_t orders_null,
    const jitjson_item_row_t *items,
    size_t item_count,
    const uint8_t *strings,
    size_t string_size,
    uint8_t strings_plain,
    uint8_t *output,
    size_t output_cap,
    size_t *written
);

void jitjson_encoder_destroy(jitjson_encoder_t *encoder);
int jitjson_cpu_supports_avx2(void);
size_t jitjson_encoder_code_size(const jitjson_encoder_t *encoder);
const char *jitjson_status_message(jitjson_status_t status);

#endif
