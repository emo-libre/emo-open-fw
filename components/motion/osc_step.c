#include <math.h>
#include "osc.h"

void osc_step(osc_t *o, osc_frame_t *out)
{
    /* Phase is derived from the frame index, not accumulated, so long runs do not drift. */
    double t = (double)o->frame * o->dphase;

    for (int j = 0; j < OSC_SERVOS; j++) {
        double v = (double)o->p.amp[j] * sin(o->p.phase0[j] + o->start_phase + t) + (double)o->p.centre[j];
        int rel = (int)v;                           /* truncation toward zero */
        if (o->p.invert[j]) {
            rel = -rel;
        }
        int ab = rel + OSC_NEUTRAL_DEG + o->p.trim[j];
        if (ab < 0) {
            ab = 0;
        } else if (ab > 180) {
            ab = 180;
        }
        out->rel[j] = rel;
        out->abs[j] = ab;
        out->us[j] = osc_angle_to_us(ab);
    }
    o->frame++;
}
