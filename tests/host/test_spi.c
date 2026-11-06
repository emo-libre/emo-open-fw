#include "test_util.h"
#include "k210_spi.h"

int main(void)
{
    uint8_t h[K210_SPI_HEADER_LEN];

    /* len 0x0400 (1024), file mode 7: sum = 0+0+0x00+0x04+7 = 0x0B */
    CHECK_EQ(k210_spi_build_header(h, 0, 0, 1024, K210_SPI_MODE_FILE_A), 0);
    const uint8_t want[] = { 0, 0, 0x00, 0x04, 7, 0x0B, 0, 0 };
    CHECK_MEM(h, want, 8);

    /* sum wraps at 8 bits; a, b participate */
    CHECK_EQ(k210_spi_build_header(h, 0xF0, 0x20, 0x01FF, 2), 0);
    CHECK_EQ(h[2], 0xFF);
    CHECK_EQ(h[3], 0x01);
    CHECK_EQ(h[5], (0xF0 + 0x20 + 0xFF + 0x01 + 2) & 0xFF);

    CHECK_EQ(k210_spi_build_header(h, 0, 0, 0xFFFF, 0), 0);
    CHECK_EQ(k210_spi_build_header(h, 0, 0, 0x10000, 0), -1);

    /* padding to 64 */
    CHECK_EQ(k210_spi_padded_len(0), 0);
    CHECK_EQ(k210_spi_padded_len(1), 64);
    CHECK_EQ(k210_spi_padded_len(64), 64);
    CHECK_EQ(k210_spi_padded_len(65), 128);
    CHECK_EQ(k210_spi_padded_len(1024), 1024);

    TEST_DONE();
}
