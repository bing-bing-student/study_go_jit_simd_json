//go:build linux && amd64 && cgo

#include "internal.h"

#include <stdlib.h>

jitjson_status_t jitjson_encoder_create(jitjson_mode_t mode, jitjson_backend_t backend, jitjson_encoder_t **out_encoder) {
    jitjson_encoder_t *encoder;

    if (out_encoder == NULL) {
        return JITJSON_ERR_INVALID_ARGUMENT;
    }
    *out_encoder = NULL;
    if (mode < JITJSON_MODE_AUTO || mode > JITJSON_MODE_AVX2 ||
        backend < JITJSON_BACKEND_JIT || backend > JITJSON_BACKEND_STATIC) {
        return JITJSON_ERR_INVALID_ARGUMENT;
    }

    encoder = (jitjson_encoder_t *)calloc(1, sizeof(*encoder));
    if (encoder == NULL) {
        return JITJSON_ERR_JIT_ALLOC;
    }
    encoder->mode = mode;
    encoder->backend = backend;

    if (mode == JITJSON_MODE_AVX2 && !jitjson_cpu_supports_avx2()) {
        free(encoder);
        return JITJSON_ERR_UNSUPPORTED_CPU;
    }
    if (mode == JITJSON_MODE_AVX2 || (mode == JITJSON_MODE_AUTO && jitjson_cpu_supports_avx2())) {
        encoder->write_string = jitjson_writer_string_avx2;
    } else {
        encoder->write_string = jitjson_writer_string_scalar;
    }

    if (backend == JITJSON_BACKEND_STATIC) {
        encoder->order_fn = jitjson_encode_order_static;
    } else {
        jitjson_status_t status = jitjson_compile_order(encoder);
        if (status != JITJSON_OK) {
            free(encoder);
            return status;
        }
    }

    *out_encoder = encoder;
    return JITJSON_OK;
}

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
) {
    static const uint8_t prefix[] = "{\"orders\":";
    jitjson_writer_t writer;
    jitjson_batch_view_t batch;
    size_t i;

    if (encoder == NULL || encoder->order_fn == NULL || written == NULL ||
        output == NULL || (order_count != 0 && orders == NULL) ||
        (item_count != 0 && items == NULL) || (string_size != 0 && strings == NULL) ||
        strings_plain > 1) {
        return JITJSON_ERR_INVALID_ARGUMENT;
    }
    if (orders_null != 0 && order_count != 0) {
        return JITJSON_ERR_INVALID_ARGUMENT;
    }

    writer.data = output;
    writer.cap = output_cap;
    writer.len = 0;
    writer.status = JITJSON_OK;

    batch.items = items;
    batch.item_count = item_count;
    batch.strings = strings;
    batch.string_size = string_size;
    batch.write_string = strings_plain != 0 ? jitjson_writer_string_plain : encoder->write_string;

    jitjson_writer_literal(&writer, prefix, sizeof(prefix) - 1);
    if (orders_null != 0) {
        jitjson_writer_literal(&writer, (const uint8_t *)"null", 4);
    } else {
        jitjson_writer_literal(&writer, (const uint8_t *)"[", 1);
        for (i = 0; i < order_count && writer.status == JITJSON_OK; i++) {
            if (i != 0) {
                jitjson_writer_literal(&writer, (const uint8_t *)",", 1);
            }
            encoder->order_fn(&writer, &orders[i], &batch);
        }
        jitjson_writer_literal(&writer, (const uint8_t *)"]", 1);
    }
    jitjson_writer_literal(&writer, (const uint8_t *)"}", 1);

    *written = writer.len;
    return writer.status;
}

size_t jitjson_encoder_code_size(const jitjson_encoder_t *encoder) {
    return encoder == NULL ? 0 : encoder->code_size;
}

void jitjson_encoder_destroy(jitjson_encoder_t *encoder) {
    if (encoder == NULL) {
        return;
    }
    jitjson_release_code(encoder);
    free(encoder);
}

const char *jitjson_status_message(jitjson_status_t status) {
    switch (status) {
        case JITJSON_OK: return "ok";
        case JITJSON_ERR_INVALID_ARGUMENT: return "invalid argument";
        case JITJSON_ERR_NO_SPACE: return "output buffer has no space";
        case JITJSON_ERR_UNSUPPORTED_CPU: return "requested CPU feature is unavailable";
        case JITJSON_ERR_JIT_ALLOC: return "JIT memory allocation failed";
        case JITJSON_ERR_JIT_PROTECT: return "JIT memory protection failed";
        case JITJSON_ERR_INTERNAL: return "internal error";
        default: return "unknown status";
    }
}
