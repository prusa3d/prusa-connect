### Changed
- The web app and API listener presents a renewed `core.api.tls` certificate within about a second of the files changing, without a restart, as the printer listener already did.
- The line `the API listener serves TLS` names the certificate's DNS names, IP addresses and issuer, as the device listener's line does, so the log shows which certificate a reload picked up.
