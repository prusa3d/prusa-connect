---
title: "ADR-20261126-qca: a printer paired at its screen is approved by its address"
description: "The server asks a printer on the local network to pair. The printer's next registration is approved with no second step when it comes from the address the pairing reached, after the person at the printer pressed yes, within two minutes, and once."
date: 2026-11-26
status: proposed
supersedes: []
amends: []
tags:
  - adr
  - devices
  - security
---
# ADR-20261126-qca: a printer paired at its screen is approved by its address

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md)

A person adds a printer by typing its address into this server's page. The
server sends a pairing request to the printer's local API. The printer shows
the question and a code, and the person at the printer presses yes. The printer
then stores this server's address and registers over the xBuddy wire, the same
`POST /p/register` it makes when a person chooses Add Printer to Connect. That
registration carries the serial number, the fingerprint, the model and the
firmware, and nothing from the pairing. Firmware has no field that could carry
a nonce from the pairing to the registration. So this record says which
registration a pairing approves without a person typing its code.

## rules

1. The server sends the pairing request, never the browser. It dials only a
   private, link-local or RFC 6598 shared address, checked after the name is
   resolved. It follows no redirect and reads no proxy from the environment.
2. The pairing records the IP address its request reached, read from the
   connection and never from the name the person typed.
3. A registration belongs to a pairing only when all of these hold:
   - it comes from the address in rule 2, read from the remote end of the
     connection. A forwarded header never stands in for it
   - the pairing is open: the person who started it did not withdraw it, and
     it did not end
   - no earlier registration was matched to the same pairing
4. A registration that matches while the question is on the screen is held.
   It is approved only when the printer answers the next poll with 200. It is
   dropped on 403 and on 404.
5. A registration that matches after the printer answered 200 is approved at
   once, so the printer's first poll of `GET /p/register` gets its token.
6. A pairing approves registrations for two minutes after the printer answered
   200. After that the registration waits for a person, as one without a
   pairing does.
7. A pairing waits 75 seconds for the person at the printer. Firmware closes
   its question after 60.
8. The approval goes through the operator's approval surface, as the person
   who started the pairing. The device is bound and owned as if that person had
   typed the code, and the same event is published.
9. A registration no pairing matches is never approved by the pairing service,
   in any admission mode.
10. The body of the printer's 200 is never parsed, stored or logged. Firmware
    running PrusaLink puts its API key there.
11. Pairing a printer takes `admit_device` on the driver resource kind, the
    permission approving a registration takes.
12. A pairing lives in the memory of one process. A restart ends it, and the
    printer's registration then waits for a person.
13. At most eight pairings are open at once, and one per printer address.

## enforcement

| rule | gate |
|---|---|
| 1 | `TestTheClientDialsOnlyTheLocalNetwork` in `internal/connector/printerpairing` |
| 2 | `TestAPrinterAsksThePersonAndSaysYesOnce` in `internal/connector/printerpairing` |
| 3 | `TestARegistrationFromAnotherAddressIsNotThePairings` in `internal/connector/printerpairing` |
| 3 | `TestAForwardedHeaderIsNotTheAddressARegistrationCameFrom` in `internal/connector/xbuddy` |
| 3 | `TestAWithdrawnPairingApprovesNothing` in `internal/connector/printerpairing` |
| 3 | `TestAPairingApprovesOneRegistration` in `internal/connector/printerpairing` |
| 4 | `TestARegistrationBeforeTheYesWaitsForIt` in `internal/connector/printerpairing` |
| 4 | `TestARegistrationBeforeACancelIsNeverApproved` in `internal/connector/printerpairing` |
| 4 | `TestAQuestionThatClosedUnansweredApprovesNothing` in `internal/connector/printerpairing` |
| 5 | `TestARegistrationIsHeardWithItsAddressBeforeTheCodeIsAnswered` in `internal/connector/xbuddy` |
| 6 | `TestARegistrationAfterTheWindowWaitsForAPerson` in `internal/connector/printerpairing` |
| 7 | `TestAQuestionNobodyAnswersRunsOut` in `internal/connector/printerpairing` |
| 8 | `TestAPrinterPairedAtItsScreenJoinsWithNoSecondStep` in `internal/app` |
| 8 | `TestARegistrationAfterTheYesIsApprovedByWhoeverPaired` in `internal/connector/printerpairing` |
| 9 | `TestARegistrationNoPairingLedToWaitsForAPerson` in `internal/app` |
| 10 | none. A reviewer reads `Client.Poll` |
| 11 | `TestNoPrinterPairingReachesACallerWhoMayNotAdmitADevice` in `internal/transport/http/httpapi` |
| 12 | none. The service has no store |
| 13 | `TestOnePrinterHasOnePairingOpen` in `internal/connector/printerpairing` |

## rejected

| option | why not |
|---|---|
| match by serial number alone | the serial is printed on the printer and sent in clear in every registration, so anything on the network can send it |
| read the serial from `/api/v1/info` with the API key from the 200 | it makes this server hold the printer's PrusaLink password, and a firmware that answers `{"paired": true}` gives no key |
| a nonce from the pairing in the registration | firmware has no field for it, and the 20 characters of the stored host have no room |
| approve any registration that arrives while a pairing is open | any machine on the network would be approved because somebody paired another printer |
| approve a registration held while waiting when the printer answers 404 | 404 is both a question that timed out and an approval whose answer was lost, and only one of them is a yes |
| the browser sends the pairing request | a page served over https cannot call `http://<printer>`, and a browser on another network cannot reach the printer at all |
| keep a pairing in the database | it lasts about a minute, and only this process can hear the printer answer it |

## Why the address is enough

The address check asks the attacker for more than the code check it replaces.
To have its own registration approved, a machine has to open a TCP connection
from the printer's IP address. It has to do that within two minutes of a person
pressing yes on that printer, and before the printer's own registration. On a
local network that takes ARP spoofing. A machine that can do that can already
read and change the printer's plain-HTTP traffic, including the code a person
would type. The person still sees the code on this server's page and on the
printer's screen, and the printer shows this server's host and port beside it.

## open items

- a server behind a reverse proxy or a NAT for the printer wire sees the proxy's
  address, and every paired registration then waits for a person. The page says
  so and lists the registration for approval
- Docker Desktop rewrites source addresses, so the same happens there
