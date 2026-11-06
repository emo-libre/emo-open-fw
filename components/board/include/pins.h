/* Board pin map. Source: spec-clean/hardware_map.md.  Tags there: [I] inferred, [V] verify on hardware. */
#ifndef EMO_PINS_H
#define EMO_PINS_H

/* K210 UART link (UART1, 1 Mbaud). GPIO12 is the MTDI strapping pin: nothing may drive it high at reset. */
#define PIN_K210_UART_TX      12
#define PIN_K210_UART_RX       4
#define K210_UART_BAUD   1000000

/* K210 bulk SPI (HSPI / SPI2_HOST), chip select driven in software */
#define PIN_K210_SPI_MOSI     15
#define PIN_K210_SPI_MISO      2
#define PIN_K210_SPI_SCLK     14
#define PIN_K210_SPI_CS       13
#define K210_SPI_HZ      8000000

/* I2C bus */
#define PIN_I2C_SDA           19
#define PIN_I2C_SCL           18
#define I2C_BUS_HZ        100000

/* Power / reset */
#define PIN_POWER_HOLD        23   /* output, high [I] */
#define PIN_K210_RESET        22   /* idle high; ~1 ms low pulse resets the K210 [I] */

/* Inputs */
#define PIN_CHARGER_PRESENT   36
#define PIN_PRESENCE_IRQ      35

/* Servo signals (later phase; listed so nothing else claims them) */
#define PIN_SERVO_0_NEW       21
#define PIN_SERVO_0_OLD        0
#define PIN_SERVO_1           25
#define PIN_SERVO_2            5
#define PIN_SERVO_3           26

/* Touch pads 7, 8, 9 (later phase) [I] */
#define PIN_TOUCH_7           27
#define PIN_TOUCH_8           33
#define PIN_TOUCH_9           32

/* Battery sense: ADC1 channel 3 == GPIO39. Divider ratio unknown [V]. */
#define BATT_ADC_GPIO         39

/* I2C device addresses (7-bit) */
#define I2C_ADDR_PRESENCE   0x71
#define I2C_ADDR_ALS_PS     0x23
#define I2C_ADDR_TOF        0x29
#define I2C_ADDR_MAG        0x30
#define I2C_ADDR_IMU_A      0x68
#define I2C_ADDR_IMU_B      0x6A
#define I2C_ADDR_IMU_C      0x6B

#endif
