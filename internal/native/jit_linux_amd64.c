//go:build linux && amd64 && cgo

#include "internal.h"

#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

typedef struct {
    uint8_t *data;
    size_t cap;
    size_t len;
    jitjson_status_t status;
} jitjson_emitter_t;

static void emit_bytes(jitjson_emitter_t *emitter, const uint8_t *data, size_t len) {
    if (emitter->status != JITJSON_OK) {
        return;
    }
    if (len > emitter->cap - emitter->len) {
        emitter->status = JITJSON_ERR_INTERNAL;
        return;
    }
    memcpy(emitter->data + emitter->len, data, len);
    emitter->len += len;
}

static void emit_u64(jitjson_emitter_t *emitter, uint64_t value) {
    emit_bytes(emitter, (const uint8_t *)&value, sizeof(value));
}

static uint64_t order_op_address(jitjson_order_op_t operation) {
    uintptr_t address = 0;
    _Static_assert(sizeof(operation) == sizeof(address), "unexpected function pointer size");
    memcpy(&address, &operation, sizeof(address));
    return (uint64_t)address;
}

static void emit_call_order_op(jitjson_emitter_t *emitter, jitjson_order_op_t operation) {
    static const uint8_t move_arguments[] = {
        0x4c, 0x89, 0xe7, /* mov rdi, r12 */
        0x4c, 0x89, 0xee, /* mov rsi, r13 */
        0x4c, 0x89, 0xf2  /* mov rdx, r14 */
    };
    static const uint8_t movabs_rax[] = {0x48, 0xb8};
    static const uint8_t call_rax[] = {0xff, 0xd0};

    emit_bytes(emitter, move_arguments, sizeof(move_arguments));
    emit_bytes(emitter, movabs_rax, sizeof(movabs_rax));
    emit_u64(emitter, order_op_address(operation));
    emit_bytes(emitter, call_rax, sizeof(call_rax));
}

jitjson_status_t jitjson_compile_order(jitjson_encoder_t *encoder) {
    static const uint8_t prologue[] = {
        0x41, 0x54,             /* push r12 */
        0x41, 0x55,             /* push r13 */
        0x41, 0x56,             /* push r14 */
        0x49, 0x89, 0xfc,       /* mov r12, rdi */
        0x49, 0x89, 0xf5,       /* mov r13, rsi */
        0x49, 0x89, 0xd6        /* mov r14, rdx */
    };
    static const uint8_t epilogue[] = {
        0x41, 0x8b, 0x44, 0x24, 0x18, /* mov eax, DWORD PTR [r12+24] */
        0x41, 0x5e,                   /* pop r14 */
        0x41, 0x5d,                   /* pop r13 */
        0x41, 0x5c,                   /* pop r12 */
        0xc3                          /* ret */
    };
    static const jitjson_order_op_t operations[] = {
        jitjson_op_order_id,
        jitjson_op_created_at,
        jitjson_op_status,
        jitjson_op_payment,
        jitjson_op_buyer,
        jitjson_op_shipping,
        jitjson_op_items,
        jitjson_op_remark
    };
    long page_size_value;
    size_t page_size;
    void *memory;
    jitjson_emitter_t emitter;
    size_t i;
    jitjson_order_fn_t function;

    if (encoder == NULL) {
        return JITJSON_ERR_INVALID_ARGUMENT;
    }
    page_size_value = sysconf(_SC_PAGESIZE);
    if (page_size_value <= 0) {
        return JITJSON_ERR_JIT_ALLOC;
    }
    page_size = (size_t)page_size_value;
    memory = mmap(NULL, page_size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (memory == MAP_FAILED) {
        return JITJSON_ERR_JIT_ALLOC;
    }

    emitter.data = (uint8_t *)memory;
    emitter.cap = page_size;
    emitter.len = 0;
    emitter.status = JITJSON_OK;

    emit_bytes(&emitter, prologue, sizeof(prologue));
    for (i = 0; i < sizeof(operations) / sizeof(operations[0]); i++) {
        emit_call_order_op(&emitter, operations[i]);
    }
    emit_bytes(&emitter, epilogue, sizeof(epilogue));

    if (emitter.status != JITJSON_OK) {
        munmap(memory, page_size);
        return emitter.status;
    }
    __builtin___clear_cache((char *)memory, (char *)memory + emitter.len);
    if (mprotect(memory, page_size, PROT_READ | PROT_EXEC) != 0) {
        munmap(memory, page_size);
        return JITJSON_ERR_JIT_PROTECT;
    }

    _Static_assert(sizeof(function) == sizeof(memory), "unexpected code pointer size");
    memcpy(&function, &memory, sizeof(function));
    encoder->code = memory;
    encoder->code_size = emitter.len;
    encoder->mapping_size = page_size;
    encoder->order_fn = function;
    return JITJSON_OK;
}

void jitjson_release_code(jitjson_encoder_t *encoder) {
    if (encoder->code != NULL && encoder->mapping_size != 0) {
        munmap(encoder->code, encoder->mapping_size);
    }
    encoder->code = NULL;
    encoder->code_size = 0;
    encoder->mapping_size = 0;
    encoder->order_fn = NULL;
}
