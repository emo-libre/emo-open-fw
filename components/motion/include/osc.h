/* Sinusoidal motion engine: four servos, angle = amplitude * sin(phase) + centre.
 * Specification: spec-clean/motion_oscillator.md. Original implementation, written from the maths and the
 * behavioural description (not derived from any third-party oscillator library). Portable: no ESP-IDF. */
#ifndef OSC_H
#define OSC_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define OSC_SERVOS      4
#define OSC_FRAME_MS    20u
#define OSC_NEUTRAL_DEG 90

typedef struct {
    int     amp[OSC_SERVOS];       /* degrees */
    int     centre[OSC_SERVOS];    /* degrees, relative to neutral */
    double  phase0[OSC_SERVOS];    /* radians */
    uint8_t invert[OSC_SERVOS];
    int     trim[OSC_SERVOS];      /* degrees, per-unit calibration */
} osc_params_t;

typedef struct {
    int      rel[OSC_SERVOS];      /* relative to neutral, after invert */
    int      abs[OSC_SERVOS];      /* rel + 90 + trim, clamped 0..180 */
    uint16_t us[OSC_SERVOS];       /* pulse width */
} osc_frame_t;

typedef struct {
    osc_params_t p;
    double   start_phase;          /* radians, applied to every servo */
    double   dphase;
    double   frames_per_cycle;
    uint32_t frame;                /* frames produced so far */
    uint32_t cycles_done;          /* whole cycles completed */
} osc_t;

/* angle (0..180, clamped) -> pulse width in microseconds */
uint16_t osc_angle_to_us(int angle_deg);

/* Prepare a run. period_ms is clamped to >= OSC_FRAME_MS (DV-009). start_phase_deg applies to all servos.
 * Returns 0, or -1 on bad arguments. */
int osc_begin(osc_t *o, const osc_params_t *p, uint32_t period_ms, double start_phase_deg);

/* Compute the next frame and advance. */
void osc_step(osc_t *o, osc_frame_t *out);

typedef struct {
    int  (*emit)(void *ctx, const osc_frame_t *f);   /* drive the servos; non-zero aborts the run with OSC_ERR */
    void (*pace)(void *ctx);                         /* wait one frame (sleep 20 ms or wait for the beat tick) */
    int  (*hold)(void *ctx);                         /* non-zero stops the run (may be NULL) */
    void (*cycle_done)(void *ctx);                   /* after each whole cycle (may be NULL) */
    void *ctx;
} osc_sink_t;

typedef enum { OSC_DONE = 0, OSC_HELD = 1, OSC_ERR = -1 } osc_result_t;

/* Run `cycles` cycles (fractional allowed, >= 0): floor(cycles) whole cycles, then the remainder, phase-continuous.
 * frames_out (optional) receives the number of frames emitted. */
int osc_run(osc_t *o, double cycles, const osc_sink_t *sink, uint32_t *frames_out);

#ifdef __cplusplus
}
#endif
#endif
