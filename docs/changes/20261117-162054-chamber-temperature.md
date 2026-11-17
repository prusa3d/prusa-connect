### Fixed
- A CORE One now reports its chamber temperature and target at /temperature/chamber and /temperature/chamber_target. An XL reports its enclosure temperature at /temperature/chamber while the enclosure is on. The printer sends both, and they were dropped on the way in.

### Added
- A CORE One now reports its chamber fans and light at /chamber/fan_pwm_target, /chamber/fan_1_rpm, /chamber/fan_2_rpm and /chamber/led_intensity. Firmware 7.0.0 and later report a new target, fan target or light within seconds, and older firmware with its next full telemetry frame.
- The chamber fan and light controls show what the printer reports. The chamber shows heating below its target, as a print waits for the bed to warm it, and cooling above it. With no target it shows off. The fans keep the air under the target only in Auto mode, and the controls say so only while a target is set.
- The printer's page shows the chamber temperature beside the bed, and a CORE One's chamber target can be set from it. The chamber also has its own row in the Controls tab and its own lines in History.

### Changed
- A chamber target above 55 C on a CORE One, or above 60 C on a CORE One L, is now refused. The firmware cuts a higher target down without saying so.
