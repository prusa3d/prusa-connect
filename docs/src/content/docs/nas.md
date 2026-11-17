---
title: Run it on a NAS
description: Running Prusa Connect on a Synology, unRAID or TrueNAS box, either as the release binary started at boot or as the published image with Docker Compose.
audience: run
rewrite:
  describes: current
  implementedBy:
    - .github/workflows/release.yml
    - config/config.yaml
    - docker/server
    - docker-compose.local.yml
---

A NAS is a Linux machine, so Prusa Connect runs on it in one of two ways.
You can start the release binary when the NAS starts, or you can run the
published image with Docker Compose. The binary needs no Docker. The image
needs no task scheduler, and Docker restarts it when the NAS starts.

| way | needs | page with the commands |
|---|---|---|
| the release binary, started at boot | a shell on the NAS, over SSH, and the NAS's own task scheduler | [Install from a release](./install-from-a-release.md) |
| the published image | Docker with Compose on the NAS | [Container deployment](./container.md#run-the-published-image) |

## Pick the binary for the NAS

Log in over SSH and run `uname -m`. `x86_64` means `prusa_connect_amd64`,
and `aarch64` means `prusa_connect_aarch64`. Most Synology, unRAID and
TrueNAS boxes are `x86_64`. Check the file against the checksum the downloads
page shows for it, as
[Install from a release](./install-from-a-release.md#check-the-download)
shows.

## Keep the folder on a share

Put the server's folder on a disk the NAS keeps, not in the system area that an
update of the NAS may replace. On unRAID, use the path on the cache pool and not
the `/mnt/user` path. `/mnt/user` passes every file through a layer that
merges the disks, and the database is not safe behind it. The server writes everything into the folder it
starts in, so one folder holds all of it:

```text
prusa-connect/
  prusa_connect_amd64
  config/config.yaml
  config/config.d/        the database: accounts, permissions, printers
  storage/                the print files you upload
```

| NAS | a folder that survives a system update |
|---|---|
| Synology DSM | a shared folder, such as `/volume1/prusa-connect` |
| unRAID | the appdata share on the cache pool, such as `/mnt/cache/appdata/prusa-connect` |
| TrueNAS SCALE | a dataset in your pool, such as `/mnt/tank/prusa-connect` |

## Start the binary when the NAS starts

Every one of these systems can run a command at boot. The command goes into
the folder and starts the server in the background, with its log in a file:

```sh
cd /volume1/prusa-connect && nohup ./prusa_connect_amd64 >> server.log 2>&1 &
```

Use your own folder in place of `/volume1/prusa-connect`.

| NAS | where a command runs at boot |
|---|---|
| Synology DSM | **Control Panel**, **Task Scheduler**, **Create**, **Triggered Task**, **User-defined script**, event **Boot-up** |
| unRAID | the **User Scripts** plugin, with the schedule **At Startup of Array** |
| TrueNAS SCALE | **System Settings**, **Advanced**, **Init/Shutdown Scripts**, type **Command**, when **Post Init** |

A boot task usually runs as `root`. The server needs no special rights, so
choose a normal user for the task where the NAS offers one, and give that user
write access to the folder.

The first start opens the first-run page on port 8080. Answer it as
[Install from a release](./install-from-a-release.md#set-it-up-in-the-wizard)
describes, then restart the server once. On a NAS the restart is a reboot, or
stopping the process and running the boot command again yourself.

## Run the image on the NAS

Where the NAS runs Docker with Compose, make a folder on a share with the
release's `config.yaml` and the example `compose.yaml` from
[Start it with Docker Compose](./container.md#start-it-with-docker-compose),
and start it over SSH. The
image is published for `x86_64` and `aarch64`, the two kinds of NAS the
binaries above cover. Compose keeps the database and the print files in named
volumes, and it mounts `config.yaml` from that folder.

CasaOS, Synology Container Manager and the unRAID Compose Manager rewrite the compose project name.
The shipped file pins the name of each volume, so they still find the same data. A compose file of
your own must pin the names the same way. Otherwise use bind mounts, and run `chown 10001:10001` on
each folder, the uid and gid the image runs as.

| NAS | Docker with Compose |
|---|---|
| Synology DSM | the **Container Manager** package. Run the commands over SSH with `sudo` |
| unRAID | Docker is built in. The **Docker Compose Manager** plugin adds Compose |
| TrueNAS SCALE | a custom app from the image `ghcr.io/prusa3d/prusa-connect`, with the settings of the example `compose.yaml` |

## Let the printers reach the NAS

The printer connects to port 8080 on the NAS. Allow that port in the NAS's own
firewall, if it runs one, for the network the printers are on. Then write the
NAS's address into the USB file, as
[Add a printer with a USB drive](./install-from-a-release.md#add-a-printer-with-a-usb-drive)
shows.
