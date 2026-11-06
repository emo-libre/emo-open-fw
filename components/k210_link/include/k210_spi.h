/* Bulk SPI link: header format and size helpers (portable). The ESP-IDF transport is in k210_port. */
#ifndef K210_SPI_H
#define K210_SPI_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define K210_SPI_HEADER_LEN 8u
#define K210_SPI_PAD        64u
#define K210_SPI_BLOCK      1024u

typedef enum {
    K210_SPI_MODE_AUDIO = 0,   /* PCM into the speaker ring        */
    K210_SPI_MODE_FRAME = 2,   /* camera/face frame (receive)      */
    K210_SPI_MODE_FILE_A = 7,  /* file data                        */
    K210_SPI_MODE_FILE_B = 8,
    K210_SPI_MODE_FILE_C = 9,
    K210_SPI_MODE_RAW   = 10   /* raw copy                         */
} k210_spi_mode_t;

/* header = a, b, len_lo, len_hi, mode, sum(bytes 0..4), 0, 0.  Returns 0, or -1 if len > 0xFFFF. */
int    k210_spi_build_header(uint8_t out[K210_SPI_HEADER_LEN], uint8_t a, uint8_t b, size_t len, uint8_t mode);
/* Body length on the wire: len rounded up to a multiple of 64 (0 stays 0). */
size_t k210_spi_padded_len(size_t len);

#ifdef __cplusplus
}
#endif
#endif
