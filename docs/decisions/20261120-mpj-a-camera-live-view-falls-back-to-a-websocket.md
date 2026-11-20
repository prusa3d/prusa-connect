---
title: "ADR-20261120-mpj: A camera live view falls back to a websocket"
description: "An H.264 live view that cannot connect over WebRTC plays over a websocket on the page's own port, and WebRTC opens the camera only once the browser connects."
date: 2026-11-20
status: proposed
supersedes: []
amends: [ADR-20261106-ypn]
tags:
  - adr
  - cameras
---
# ADR-20261120-mpj: A camera live view falls back to a websocket

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Amends [ADR-20261106-ypn](20261106-ypn-a-camera-is-one-generic-type-with-a-still-source-and-an.md) | Related: [ADR-20261119-pxn](20261119-pxn-a-camera-live-view-plays-while-it-is-seen.md)

ADR-20261106-ypn plays an H.264 camera over WebRTC. The video then goes over
UDP, straight between the browser and the server. A browser that cannot reach
the server on UDP sees no video. That happens behind a tunnel that carries no
UDP, and in a container with no published UDP port.

The page itself always reaches the server, on its HTTPS port. A websocket on
that port carries the same pictures. It costs some latency on a bad network,
because TCP resends what UDP would drop, so it is the fallback and not the
first try.

## rules

1. An H.264 live view tries WebRTC first. When the connection is not up five
   seconds after the answer, the view falls back to the live socket,
   `GET /api/v1/cameras/{camera}/live`.
2. Once one view on a page has fallen back, every later view on that page goes
   to the live socket first. The server's candidates are the same for every
   camera, so the next view would fail the same way.
3. The live socket carries one picture per binary message: a flag byte, 1 for
   a keyframe, then the access unit in Annex-B framing. The first message is a
   keyframe, and every keyframe carries the parameter sets.
4. The browser decodes the socket with WebCodecs. A browser without WebCodecs
   says the view did not start, as ADR-20261119-pxn rule 3 holds. A picture
   that does not decode costs the pictures up to the next keyframe, not the
   view. The view ends when five decoders fail in a row with nothing drawn.
5. A viewer about a second behind skips to the next keyframe, on the server
   and in the browser.
6. The live socket opens the camera before the upgrade, so its refusals are
   the same HTTP answers as the WebRTC path's. It refuses an Origin that is not
   the request's own host.
7. The WebRTC relay opens the camera only once the browser connects. A view
   that never connects holds none of the camera's stream slots.

## enforcement

The `cameralive_test.go` tests hold rules 3, 5 and 6 on the server, and
`TestEveryKeyframeOnTheLiveSocketCarriesTheParameterSets` holds rule 3's
parameter sets. `TestAViewerThatNeverDialsNeverOpensTheCamera` holds rule 7.
The `PrinterCamerasPanel` tests hold rules 1 and 2, the `LiveDecoder` tests
hold rule 4, and the `h264` tests hold the framing the browser reads.

## rejected

| option | why not |
|---|---|
| the websocket as the only path | a picture stalls on a lossy network, where WebRTC skips |
| fragmented MP4 through Media Source Extensions | a container built per picture, and no MSE on an iPhone before iOS 17.1 |
| a streamed HTTP response | a buffering proxy holds it, and `fetch` gives no message boundaries |
| a TURN relay | one more server to run, and a tunnel still carries no UDP |
| opening the camera before the WebRTC answer | a failed attempt holds a stream slot, and its fallback waits behind it |
