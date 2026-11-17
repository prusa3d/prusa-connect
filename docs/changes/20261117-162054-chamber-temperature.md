### Fixed
- A CORE One now reports its chamber temperature and target at /temperature/chamber and /temperature/chamber_target, and an XL reports its enclosure temperature at /temperature/chamber. The printer sends both, and they were dropped on the way in.

### Added
- A CORE One now reports its chamber fans and light at /chamber/fan_pwm_target, /chamber/fan_1_rpm, /chamber/fan_2_rpm and /chamber/led_intensity. A new target or light shows at once, although the printer reports it only in its next full telemetry frame.
- The chamber fan and light controls show what the printer reports, and the chamber shows cooling above its target and never shows heating. The chamber has fans and no heater. Its target is a ceiling, and the fans keep the air under it only in Auto mode.
- The printer's page shows the chamber temperature beside the bed, and a CORE One's chamber target can be set from it. The chamber also has its own row in the Controls tab and its own lines in History.

### Changed
- A chamber target above 55 C on a CORE One, or above 60 C on a CORE One L, is now refused. The firmware cuts a higher target down without saying so.
