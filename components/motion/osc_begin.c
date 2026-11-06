#include <math.h>
#include <string.h>
#include "osc.h"

#define TWO_PI 6.28318530717958647692

int osc_begin(osc_t *o, const osc_params_t *p, uint32_t period_ms, double start_phase_deg)
{
    if (!o || !p || !isfinite(start_phase_deg)) {
        return -1;
    }
    for (int j = 0; j < OSC_SERVOS; j++) {
        if (!isfinite(p->phase0[j])) {
            return -1;
        }
    }
    memset(o, 0, sizeof *o);
    o->p = *p;
    for (int j = 0; j < OSC_SERVOS; j++) {
        o->p.invert[j] = p->invert[j] ? 1u : 0u;
    }
    if (period_ms < OSC_FRAME_MS) {
        period_ms = OSC_FRAME_MS;                   /* DV-009 */
    }
    o->frames_per_cycle = (double)period_ms / (double)OSC_FRAME_MS;
    o->dphase = TWO_PI / o->frames_per_cycle;
    o->start_phase = start_phase_deg / 360.0 * TWO_PI;
    return 0;
}
