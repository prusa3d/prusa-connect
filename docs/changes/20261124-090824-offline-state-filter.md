### Fixed
- The State filter on the Printers page now lists a switched-off printer under Offline instead of under the last state it reported. In `GET /api/v1/devices`, such a device now matches `state=OFFLINE` and is counted and sorted under `OFFLINE`, while its `state` field keeps what it last reported.
