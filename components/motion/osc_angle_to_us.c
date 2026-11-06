#include "osc.h"

/* trunc(angle * 2000 / 180 + 500 + 0.5), in integer arithmetic. angle is clamped to 0..180 (DV-010). */
uint16_t osc_angle_to_us(int angle_deg)
{
    if (angle_deg < 0) {
        angle_deg = 0;
    } else if (angle_deg > 180) {
        angle_deg = 180;
    }
    return (uint16_t)(500 + (angle_deg * 2000 + 90) / 180);
}
