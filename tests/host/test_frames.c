#include "test_util.h"
#include "k210_proto.h"

int main(void)
{
    /* short frame, hand-computed: 0x55+0xAA+0x34 = 0x133 -> 0x33 */
    uint8_t f[K210_SHORT_LEN];
    k210_build_short(f, 52, 0, 0, 0);
    const uint8_t want_reset[] = { 0x55, 0xAA, 0x34, 0x00, 0x00, 0x00, 0x33 };
    CHECK_MEM(f, want_reset, 7);

    /* volume 30: 0xFF+0x17+0x1E = 0x134 -> 0x34 */
    k210_build_short(f, 23, 30, 0, 0);
    const uint8_t want_vol[] = { 0x55, 0xAA, 0x17, 0x1E, 0x00, 0x00, 0x34 };
    CHECK_MEM(f, want_vol, 7);

    /* 16-bit argument split */
    k210_build_short(f, 56, 0x34, 0x12, 0);
    CHECK_EQ(f[3], 0x34);
    CHECK_EQ(f[4], 0x12);
    CHECK_EQ(f[6], (0xFF + 56 + 0x34 + 0x12) & 0xFF);

    /* payload frame: sub 2, body "[0,[]]\0" */
    const uint8_t body[] = { '[', '0', ',', '[', ']', ']', 0 };
    uint8_t out[64];
    size_t n = k210_build_payload(out, sizeof out, 2, body, sizeof body);
    CHECK_EQ(n, 7 + sizeof body + 1);
    CHECK_EQ(out[0], 0x55);
    CHECK_EQ(out[1], 0xAA);
    CHECK_EQ(out[2], 0xFF);
    CHECK_EQ(out[3], sizeof body);          /* len_lo */
    CHECK_EQ(out[4], 0);                    /* len_hi */
    CHECK_EQ(out[5], 2);                    /* sub */
    CHECK_EQ(out[6], (0xFF + 0xFF + sizeof body + 2) & 0xFF);   /* 0x55+0xAA = 0xFF */
    CHECK_MEM(out + 7, body, sizeof body);
    CHECK_EQ(out[7 + sizeof body], k210_crc8_maxim(body, sizeof body));

    /* empty payload: header + CRC(empty)=0 */
    n = k210_build_payload(out, sizeof out, 9, NULL, 0);
    CHECK_EQ(n, 8);
    CHECK_EQ(out[7], 0);

    /* 16-bit length split */
    static uint8_t big[600];
    static uint8_t bigout[700];
    memset(big, 0x5A, sizeof big);
    n = k210_build_payload(bigout, sizeof bigout, 7, big, 600);
    CHECK_EQ(n, 608);
    CHECK_EQ(bigout[3], 600 & 0xFF);
    CHECK_EQ(bigout[4], 600 >> 8);

    /* rejects */
    CHECK_EQ(k210_build_payload(out, 10, 2, body, sizeof body), 0);   /* too small */
    CHECK_EQ(k210_build_payload(NULL, 10, 2, body, 1), 0);
    CHECK_EQ(k210_build_payload(out, sizeof out, 2, NULL, 3), 0);
    CHECK_EQ(k210_build_payload(out, sizeof out, 2, body, 0x10000), 0);

    TEST_DONE();
}
