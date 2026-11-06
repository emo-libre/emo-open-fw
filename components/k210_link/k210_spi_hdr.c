#include "k210_spi.h"

int k210_spi_build_header(uint8_t out[K210_SPI_HEADER_LEN], uint8_t a, uint8_t b, size_t len, uint8_t mode)
{
    if (len > 0xFFFFu) {
        return -1;
    }
    uint8_t lo = (uint8_t)(len & 0xFFu);
    uint8_t hi = (uint8_t)(len >> 8);
    out[0] = a;
    out[1] = b;
    out[2] = lo;
    out[3] = hi;
    out[4] = mode;
    out[5] = (uint8_t)(a + b + lo + hi + mode);
    out[6] = 0;
    out[7] = 0;
    return 0;
}

size_t k210_spi_padded_len(size_t len)
{
    return (len + (K210_SPI_PAD - 1u)) / K210_SPI_PAD * K210_SPI_PAD;
}
