//go:build linux && amd64 && cgo

#include "internal.h"

#include <immintrin.h>

size_t jitjson_scan_special_scalar(const uint8_t *data, size_t len) {
    size_t i;
    for (i = 0; i < len; i++) {
        uint8_t value = data[i];
        if (value < 0x20 || value == '"' || value == '\\') {
            return i;
        }
    }
    return len;
}

__attribute__((target("avx2")))
size_t jitjson_scan_special_avx2(const uint8_t *data, size_t len) {
    const __m256i high_mask = _mm256_set1_epi8((char)0xe0);
    const __m256i zero = _mm256_setzero_si256();
    const __m256i quote = _mm256_set1_epi8('"');
    const __m256i slash = _mm256_set1_epi8('\\');
    size_t i = 0;

    for (; i + 32 <= len; i += 32) {
        __m256i bytes = _mm256_loadu_si256((const __m256i *)(const void *)(data + i));
        __m256i controls = _mm256_cmpeq_epi8(_mm256_and_si256(bytes, high_mask), zero);
        __m256i quotes = _mm256_cmpeq_epi8(bytes, quote);
        __m256i slashes = _mm256_cmpeq_epi8(bytes, slash);
        __m256i special = _mm256_or_si256(controls, _mm256_or_si256(quotes, slashes));
        unsigned int mask = (unsigned int)_mm256_movemask_epi8(special);
        if (mask != 0) {
            return i + (size_t)__builtin_ctz(mask);
        }
    }

    return i + jitjson_scan_special_scalar(data + i, len - i);
}

int jitjson_cpu_supports_avx2(void) {
    __builtin_cpu_init();
    return __builtin_cpu_supports("avx2") != 0;
}
