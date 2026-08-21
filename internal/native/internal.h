#ifndef JITJSON_INTERNAL_H
#define JITJSON_INTERNAL_H

#include "jitjson.h"

#include <stddef.h>
#include <stdint.h>

typedef struct {
    uint8_t *data;
    size_t cap;
    size_t len;
    jitjson_status_t status;
} jitjson_writer_t;

struct jitjson_batch_view;
typedef struct jitjson_batch_view jitjson_batch_view_t;

typedef void (*jitjson_string_writer_t)(
    jitjson_writer_t *writer,
    const jitjson_batch_view_t *batch,
    jitjson_string_ref_t ref
);

typedef jitjson_status_t (*jitjson_order_fn_t)(
    jitjson_writer_t *writer,
    const jitjson_order_row_t *order,
    const jitjson_batch_view_t *batch
);

typedef void (*jitjson_order_op_t)(
    jitjson_writer_t *writer,
    const jitjson_order_row_t *order,
    const jitjson_batch_view_t *batch
);

struct jitjson_batch_view {
    const jitjson_item_row_t *items;
    size_t item_count;
    const uint8_t *strings;
    size_t string_size;
    jitjson_string_writer_t write_string;
};

struct jitjson_encoder {
    void *code;
    size_t code_size;
    size_t mapping_size;
    jitjson_order_fn_t order_fn;
    jitjson_string_writer_t write_string;
    jitjson_mode_t mode;
    jitjson_backend_t backend;
};

void jitjson_writer_literal(jitjson_writer_t *writer, const uint8_t *data, size_t len);
void jitjson_writer_int64(jitjson_writer_t *writer, int64_t value);
void jitjson_writer_bool(jitjson_writer_t *writer, uint8_t value);
void jitjson_writer_string_plain(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref);
void jitjson_writer_string_scalar(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref);
void jitjson_writer_string_avx2(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref);
void jitjson_writer_nullable_string(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, jitjson_string_ref_t ref, uint8_t present);
void jitjson_writer_items(jitjson_writer_t *writer, const jitjson_batch_view_t *batch, uint32_t start, uint32_t count, uint8_t items_null);

void jitjson_op_order_id(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_created_at(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_status(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_payment(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_buyer(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_shipping(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_items(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
void jitjson_op_remark(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);

jitjson_status_t jitjson_encode_order_static(jitjson_writer_t *writer, const jitjson_order_row_t *order, const jitjson_batch_view_t *batch);
jitjson_status_t jitjson_compile_order(jitjson_encoder_t *encoder);
void jitjson_release_code(jitjson_encoder_t *encoder);

size_t jitjson_scan_special_scalar(const uint8_t *data, size_t len);
size_t jitjson_scan_special_avx2(const uint8_t *data, size_t len);

#endif
