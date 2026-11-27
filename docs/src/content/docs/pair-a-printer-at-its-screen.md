---
title: Pair a printer at its screen
description: Add a Prusa printer on your network by typing its address in the web app and pressing Yes on the printer, with no USB drive and no code to type.
audience: run
rewrite:
  describes: current
  implementedBy:
    - internal/connector/printerpairing
    - internal/app/printerpairings.go
    - packages/svelte/src/shell/add-printer
---

You can add a printer without a USB drive. You type the printer's address on
the **Add a printer** page. The printer then asks on its own screen whether to
pair with this server, and you press **Yes** there. The printer stores this
server's address and joins. You do not type a code anywhere.

The printer needs Buddy firmware with pairing in its local API. A printer with
older firmware is refused with a message, and the page still offers the
settings file. See [Printer configuration](printer-configuration.md) for that
way.

## What you need

| what | why |
|---|---|
| the printer's IP address | the printer shows it under **Settings**, **Network** |
| the printer and this server on one local network | this server sends the request to the printer, so it has to reach it |
| the printer idle, with no dialog on its screen | a printer that prints or shows a dialog answers that it is busy |
| a person at the printer | only a press of **Yes** on the printer lets it pair |
| the `admit_device` permission | pairing a printer admits it to this server |

## Pair the printer

1. Open **Printers**, then **Add a printer**.
2. Check the server address under **The printer will connect to**. It is the
   address the settings file would carry. Change it when the printer reaches
   this server at another address.
3. Under **Printers on this network**, find the printer and choose **Pair**.
   Type part of its name or IP address in the search box to narrow the list.
4. A printer that is not listed is added under **Add by IP address**. Type its
   address, for example `192.168.1.20`, and choose **Ask the printer**. Add a
   port, such as `192.168.1.20:8080`, when the printer's web page is not on
   port 80.
5. The page shows a code, such as `K7QX-29FM`. Go to the printer. It shows the
   question, this server's address and the same code. Check that the code
   matches, then press **Yes**.

The printer registers with this server at once. The page moves to **Printer
connected** when the printer has joined.

## Which printers the list shows

The list shows the printers that announce themselves on the local network
over mDNS, the protocol a slicer uses to find printers. This server reads that
list every 15 seconds.

A Buddy printer announces its name, its address and its port, and nothing
else. It announces no serial number and no model. So the list shows the model
only when the printer still has the name the firmware gives a new printer,
such as `prusa-xl`.

The list leaves out a printer that connects to this server from the same
address, or that last connected from it. It also leaves out another Prusa
Connect server. A printer that waits for approval is still listed, because its
registration carries no address to compare.

A printer is not listed when this server runs in a container on a bridge
network, because mDNS does not cross the bridge. Add it by its IP address.

## What happens after Yes

When you press **Yes**, the printer stores this server's host, port and TLS
setting. It then registers over the printer connection, as it does when you
choose **Add Printer to Connect** in its menu.

This server approves that registration without asking you again. It does so
only when the registration comes from the address the pairing reached, within
two minutes of the **Yes**, and only for one registration. A registration that
does not match waits on the **Add a printer** page for you to approve its code,
as any other registration does.
[ADR-20261126-qca](../../../decisions/20261126-qca-a-printer-paired-at-its-screen-is-approved-by-its-address.md)
has the rules.

## Messages the page can show

| message | what to do |
|---|---|
| The printer is printing or shows another dialog | wait until the printer is idle and its screen is clear, then try again |
| The printer already shows a pairing question | answer or cancel it on the printer, or wait a minute |
| The printer takes no pairing request for a while | wait the time the page gives, at most five minutes |
| Pairing is off on the printer until it restarts | two cancels in a row turned it off. Restart the printer |
| The printer's firmware cannot pair | update the firmware, or use the settings file |
| The printer refused the pairing request | the server address is the usual cause. Use this server's IP address |
| This server got no answer from the printer | check the address on the printer's network screen |
| This server pairs only with a printer on the local network | type a private address such as `192.168.1.20` |

A printer locks pairing for five minutes after a cancel, after a question
nobody answered, and after three requests in five minutes.

## Server address length

The printer stores at most 20 characters of the server's host name. A name
that ends in `prusa3d.com` may be longer. The page warns you when the name is
too long, and the printer refuses such a name. Use this server's IP address
instead. [Hostname length](printer-configuration.md#hostname-length) has the
full rule.

## When the printer joins but waits for approval

Sometimes the printer registers, but its code still waits on the **Add a
printer** page. This happens when this server sees the printer's connection
from another address than the one it paired with:

- the server runs behind a reverse proxy or a NAT for the printer connection
- the server runs in Docker Desktop, which changes the source address

Approve the code on the page. The printer shows the same code on its screen.
