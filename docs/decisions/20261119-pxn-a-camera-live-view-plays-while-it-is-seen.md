---
title: "ADR-20261119-pxn: A camera live view plays while it is seen"
description: "Wherever the web app draws a camera, its live view starts while its picture is on screen in a shown browser tab, and stops when it is not, unless the person stopped it."
date: 2026-11-19
status: proposed
supersedes: []
amends: [ADR-20261106-xm4]
tags:
  - adr
  - cameras
---
# ADR-20261119-pxn: A camera live view plays while it is seen

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Amends [ADR-20261106-xm4](20261106-xm4-a-printer-page-has-a-cameras-tab-and-a-module-draws-under.md)

ADR-20261106-xm4 rule 3 starts a live view only when a person presses play.
A person who opens a camera's tab has come to watch it, so the press is one
step that everybody takes. The record rejected a view that starts when the tab
opens, because every camera would then stream to every open page for as long
as it stayed open.

That cost comes from a view nobody looks at. A view that runs only while
somebody can see it costs the camera nothing while nobody does.

## rules

1. A camera's live view starts when its picture is on screen in a browser tab
   that is shown. That holds on the Cameras tab, the Now tab, the printers
   page and the dashboard's camera tile. The view stops when the picture
   leaves the screen or the tab is hidden, and starts again when both are true
   again.
2. A person who presses stop holds the view stopped. It does not start again
   by itself until they press play.
3. A live view that does not start is held the same way. The picture says it
   did not start, and the person presses play to try again. A view is never
   retried by itself.
4. The printers page and the dashboard play too, though they can draw many
   cameras at once. Each picture costs only while it is seen. A page left open
   on a wall is a wall somebody watches.
5. Rule 3 of ADR-20261106-xm4 otherwise holds. The still frame stays under
   the live view until the first frame covers it. The picture says it is live
   only while it plays.
6. A live view keeps the last frame it drew for as long as the page is open.
   That frame is shown grayed out while a view of the camera starts, and in
   place of a still the camera does not keep, so the picture never goes
   blank.
7. A live view that stops because its picture is not seen keeps its
   connection for 30 seconds. That covers leaving the screen, a hidden browser
   tab and another tab of the page. A view of the camera that starts in that
   time takes the connection over and plays at once. A stop the person pressed
   hangs up at once.

## enforcement

The `whileSeen` tests hold rule 1: on screen and shown, each change reported
once, and nothing reported after the picture is gone. The `PrinterCamerasPanel`
tests hold rules 2 and 3. The `PrinterFrame` and printers page tests hold rule
1 for the printers page, and the `PrinterCamerasPanel` tests hold rules 6
and 7. The `SessionPool` tests hold rule 7's 30 seconds.

## rejected

| option | why not |
|---|---|
| a live view that starts when the tab opens and runs until the page closes | the cost ADR-20261106-xm4 rejected: a stream for every page left open in a background tab |
| a setting each person turns on | everybody who opens the tab came to watch, so the setting would be one more step that everybody takes |
| a view that retries by itself after it did not start | a camera that is down would be asked again and again by every page that shows it |
| the play button kept on the dashboard and the printers page | a person who puts cameras on a page puts them there to watch, and each picture already stops when it is not seen |
