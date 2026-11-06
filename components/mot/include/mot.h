/* .mot body-motion files: 20-byte frame parse/validate/build and the 0xFE-move player.
 * Specification: spec-clean/mot_format.md. Portable: no ESP-IDF. */
#ifndef MOT_H
#define MOT_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define MOT_FRAME_LEN  20u
#define MOT_SUB_MOVE   0xFEu
#define MOT_SERVOS     4
#define MOT_STEP_MS    20u

typedef struct {
    uint8_t  type;
    uint8_t  sub;
    uint8_t  params[5];
    uint8_t  angle[MOT_SERVOS];
    uint8_t  b15;
    uint16_t dur_ms;
} mot_frame_t;

typedef enum {
    MOT_OK = 0,
    MOT_ERR_ARG = -1,
    MOT_ERR_SIZE = -2,
    MOT_ERR_MAGIC = -3,
    MOT_ERR_SUM = -4,
    MOT_ERR_END = -5
} mot_err_t;

mot_err_t mot_frame_parse(const uint8_t raw[MOT_FRAME_LEN], mot_frame_t *out);
void      mot_frame_build(uint8_t raw[MOT_FRAME_LEN], const mot_frame_t *f);
/* Whole-file check. On failure *bad_index (optional) is the first bad frame (or 0 for a size error). */
mot_err_t mot_validate(const uint8_t *buf, size_t len, size_t *nframes, size_t *bad_index);

/* ---- player ---- */
typedef struct {
    int      abs[MOT_SERVOS];      /* rounded absolute angle incl. trim, 0..180 */
    uint16_t us[MOT_SERVOS];
} mot_out_t;

typedef struct {
    int  (*emit)(void *ctx, const mot_out_t *o);                 /* non-zero aborts with MOT_PLAY_ERR */
    void (*pace)(void *ctx);                                     /* wait one 20 ms step */
    int  (*hold)(void *ctx);                                     /* non-zero stops (may be NULL) */
    int  (*procedural)(void *ctx, const mot_frame_t *f);         /* non-0xFE frames (may be NULL = skip) */
    void *ctx;
} mot_sink_t;

typedef struct {
    double last[MOT_SERVOS];       /* last commanded angle, degrees; starts at 90 */
    int    trim[MOT_SERVOS];       /* per-unit calibration, degrees */
} mot_player_t;

typedef enum { MOT_PLAY_DONE = 0, MOT_PLAY_HELD = 1, MOT_PLAY_BAD_FILE = -1, MOT_PLAY_ERR = -2 } mot_play_t;

void mot_player_init(mot_player_t *p, const int trim[MOT_SERVOS] /* may be NULL */);
/* Validate, then play every frame. frames_out (optional) = pulse updates emitted. */
int  mot_play(mot_player_t *p, const uint8_t *buf, size_t len, const mot_sink_t *sink, uint32_t *frames_out);

#ifdef __cplusplus
}
#endif
#endif
