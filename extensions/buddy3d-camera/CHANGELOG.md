# Changelog

What changed in this module, newest first. The top heading is the version
`module.yaml` declares and the version this process reports to the host, and
`changelog_test.go` fails when the three differ.

## 0.8.1 - 2026-11-24

### Changed

- The build tooling takes minimatch 11 for GHSA-g3hv-75wg-m249. Nothing the extension ships changes.

## 0.8.0 - 2026-11-16

### Added

- While a camera is connected, the section under it has three tabs: Control, Settings and Information.
- Control turns the camera by a step you choose, from 1° to 90°, and sets its infrared mode and its video quality: SD, HD or Full HD. The live view asks for that quality.
- Settings chooses the one stream the camera runs: WebRTC, which the live view plays, RTSP for a video player, with its address to copy, or none. It also starts and stops a timelapse on the camera's SD card, sets the timelapse interval and sets the speaker volume. Each shows only on a camera that says it has it.
- Information shows the camera's name, which you can change, and its model, firmware, hardware, uptime and temperature. It also shows its network, with the IPv6 and MAC addresses, and its SD card.
- A camera that can restart is restarted from its section, after you confirm.
- The Wi-Fi is optional when you pair a camera. Under **Set wifi credentials**, `sdcard_cfg.txt` carries the name and password you type. Unchecked, it carries the token alone, and the camera keeps its Wi-Fi.
- Pairing has a **Use a custom time server** checkbox. Checked, it asks for a time server, a host with no port since the camera asks port 123 only, which `srv_cfg.txt` names.

### Changed

- A camera needs firmware 3.1.7 or later, the stable release, in place of 3.1.7-rc8.
- The module lives in the Prusa Connect repository, under `extensions/buddy3d-camera`, and its module path is `github.com/prusa3d/buddy3d-camera`. The namespace becomes `com.github.aux.prusa3d.buddy3d-camera`, and the camera type changes with it. Remove the old module, install this one, then add each camera on its printer and pair it again.

### Removed

- The module's time server on UDP port 12300, the `ntp_server` setting and the check that the time server answers. `srv_cfg.txt` names a time server only when pairing does, so a camera keeps its own.
- The warning that a camera's clock is not set. The flag it read says whether the camera's time zone is set, not its clock.

## 0.7.0

### Added

- The module is the cameras' time server, on UDP port 12300, and `srv_cfg.txt` names it with its port. A camera does not work without the right time, so this works on a network with no internet. The new setting `ntp_server` names another time server instead, on the internet or on your network, with a port or without.
- The module checks every minute that the time server answers. When it does not, the settings page and the camera's setup say so, before anyone sets up a camera. A camera that reports its clock is not set says so too.

### Changed

- A camera is set up from one file, `local_cfg.tar`, on its SD card, and needs firmware 3.1.7-rc8 or later. The archive holds the Wi-Fi and the token as well. Your browser makes it, and the Wi-Fi password does not reach the server. The QR code and the separate `srv_cfg` and `self-signed.pem` downloads are gone.
- A camera sends a picture at most every 10 seconds, as its firmware does. An interval under 10 seconds is refused, and the 5-second choice is gone.

## 0.6.2

### Fixed

- The arrows and the infrared picker under a camera no longer vanish when the module restarts. The module asks a camera what it can do each time it connects, and keeps the answer.

## 0.6.1

### Changed

- Pairing a camera always makes a new token. The field for a token the camera already held is gone; pair the camera again, with the new QR code, to replace it.

## 0.6.0

### Changed

- The module lives at the path of the repository it is released from. Its namespace follows, and so does the camera type it offers. Install it under the new path, and add its cameras again: a camera declared with the old type is no longer one this module offers.

## 0.5.0

### Added

- A camera that can turn is turned with arrows under the camera, one 10° step a press, and its infrared mode is chosen there: automatic, off or on. The camera answers whether it did it, and a refusal shows with the camera's code. It takes a person who may operate the camera, while the camera is connected.

## 0.4.0

### Added

- How often each camera sends a picture can be chosen under the camera, or switched off. A connected camera is told at once, and one that is offline is told when it connects. It takes a person who may operate the camera.

## 0.3.0

### Added

- The live view: play on a Buddy3D camera's picture opens a WebRTC connection straight from the camera to the browser. The module carries the signalling over the camera's session and none of the video.
- The module offers the camera type Buddy3D camera, which a person picks when adding a camera.

### Changed

- A camera is set up on a camera of the Buddy3D camera type, not on a push camera. The core grants the module its cameras, so pairing writes no grant.

## 0.2.0

### Changed

- The camera's status, its setup and the button that asks it for a picture are under the camera in a printer's Cameras tab. They are in the core's `camera-section` slot, in place of a section on the Status tab.
- A camera is set up on a push camera added in the core's Cameras card. The module no longer declares the push camera itself, and forgetting a camera leaves the push camera for that card to remove.

## 0.1.1

### Fixed

- A camera port still held by the process the module replaces is tried again every few seconds, instead of staying closed until the module restarts.

## 0.1.0

### Added

- Takes a Buddy3D camera's pictures over HTTPS and hands each one to a push camera of the core with `camera.accept`.
- Pairs a camera on a printer's page: the push camera, the grant, the token, the QR code, `srv_cfg` and the certificate.
- Sets the camera's snapshot interval over its Socket.IO session, and asks for a picture on demand.
