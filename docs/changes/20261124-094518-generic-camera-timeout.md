### Fixed

- A generic camera now takes a `timeout` setting, so a slow RTSP source such as go2rtc can be given longer than 5 seconds to answer. The same setting bounds one fetch from `snapshot_url`, and leaving it out keeps the 5 second default.
