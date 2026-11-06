#include "test_util.h"
#include "k210_proto.h"

/* Independent bit-by-bit reference (generic reflected CRC-8, poly 0x31 reflected = 0x8C). */
static uint8_t ref_crc8_maxim(const uint8_t *d, size_t n)
{
    uint8_t crc = 0;
    for (size_t i = 0; i < n; i++) {
        for (int b = 0; b < 8; b++) {
            uint8_t bit = (uint8_t)(((crc ^ (d[i] >> b)) & 1u));
            crc >>= 1;
            if (bit) {
                crc ^= 0x8Cu;
            }
        }
    }
    return crc;
}

int main(void)
{
    /* Published check value for CRC-8/MAXIM-DOW */
    CHECK_EQ(k210_crc8_maxim((const uint8_t *)"123456789", 9), 0xA1);
    CHECK_EQ(k210_crc8_maxim((const uint8_t *)"", 0), 0x00);
    CHECK_EQ(k210_crc8_maxim(NULL, 0), 0x00);

    uint8_t buf[300];
    for (int round = 0; round < 200; round++) {
        size_t n = rnd() % sizeof buf;
        for (size_t i = 0; i < n; i++) {
            buf[i] = (uint8_t)rnd();
        }
        CHECK_EQ(k210_crc8_maxim(buf, n), ref_crc8_maxim(buf, n));
    }

    /* additive sum */
    uint8_t a[] = { 0xFF, 0x02, 0x03 };
    CHECK_EQ(k210_sum8(a, 3), 0x04);
    CHECK_EQ(k210_sum8(a, 0), 0x00);

    TEST_DONE();
}
