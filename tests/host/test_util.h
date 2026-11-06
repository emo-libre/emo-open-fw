#ifndef TEST_UTIL_H
#define TEST_UTIL_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int g_checks = 0;
static int g_failed = 0;

#define CHECK(cond) do { g_checks++; if (!(cond)) { g_failed++; \
    fprintf(stderr, "%s:%d: CHECK failed: %s\n", __FILE__, __LINE__, #cond); } } while (0)

#define CHECK_EQ(a, b) do { long long _a = (long long)(a), _b = (long long)(b); g_checks++; \
    if (_a != _b) { g_failed++; fprintf(stderr, "%s:%d: %s == %s  (%lld vs %lld)\n", \
    __FILE__, __LINE__, #a, #b, _a, _b); } } while (0)

#define CHECK_MEM(a, b, n) do { g_checks++; if (memcmp((a), (b), (n)) != 0) { g_failed++; \
    fprintf(stderr, "%s:%d: memory differs: %s vs %s\n", __FILE__, __LINE__, #a, #b); } } while (0)

#define TEST_DONE() do { printf("%s: %d checks, %d failed\n", __FILE__, g_checks, g_failed); \
    return g_failed ? 1 : 0; } while (0)

/* deterministic PRNG so failures reproduce */
static unsigned long g_seed = 12345UL;
static inline unsigned rnd(void) { g_seed = g_seed * 1103515245UL + 12345UL; return (unsigned)((g_seed >> 16) & 0x7FFF); }

#endif
