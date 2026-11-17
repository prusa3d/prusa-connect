---
title: Upgrade, back up and uninstall
description: Replacing the binary with a new release, copying the database and the print files, restoring them, and removing the server, for a folder install, the systemd service and Docker.
audience: run
rewrite:
  describes: current
  implementedBy:
    - deploy/systemd/prusa-connect.service
    - docker/server
    - config/config.yaml
---

Your printers, accounts and uploaded files are in a few files beside the
server, and the server is one binary. The Settings → Server → **Updates**
page can verify a signed release and rehearse its migrations on snapshots. It
can also create a consistent online backup and apply or schedule the release.
[Where your data lives](./install-from-a-release.md#where-your-data-lives)
lists the underlying files.

## Online updates and the Beta Portal

Online updates come from the Beta Portal, Prusa's beta programme for new
features and experimental builds. The server pairs with it first. On Settings →
Server → **Updates**, start the pairing and open the portal with the code.
Confirm the code with your tester account, and the pairing finishes. The server
then holds a token bound to its own key, and the browser never sees either. Both
are in `beta-portal-pairing.json`, in the folder of the file that
[`core.overlay`](./config/server.md) names. Release 0.0.1-alpha.7 wrote the file
at every start, paired or not, in `config.d` beside the configuration file. A
server that ran that release keeps using the file there while it can use it.
When the server cannot use that file, it names the file in its log and opens a
new, unpaired file beside the overlay. Pair again on **Updates**, or repair the
file and restart the server. An upgrade, a rollback and a restored backup then
all use the same file.

While the page is open, the server asks the portal once a minute whether the
pairing still works. A pairing removed in the portal shows as revoked, and
you pair again. When the team suspends your access to the product, the
pairing stays and the page says so. Updates work again once the team
restores the access.

| what | a folder install | the systemd service |
|---|---|---|
| your configuration | `config/config.yaml` | `/etc/prusa-connect/config.yaml` |
| the database | `config/config.d/` | `/var/lib/prusa-connect/config/config.d/` |
| the print files | `storage/` | `/var/lib/prusa-connect/storage/` |

Do not copy a running SQLite database manually. Use the page or
`prusa-connect backup create`: it uses SQLite's online snapshot support,
checks every database, hashes every file and records the schema ledger in a
private manifest. `backup list` and `backup restore <id>` inspect and restore
those snapshots. Stop the server before restore.

A backup holds the password hashes of every account, so each command below
makes an archive that only its owner can read. On Linux and macOS that is
`umask 077`, which makes a new file private to the user who writes it.

## Back up before an upgrade

Make a backup before every upgrade. A new release may change the database to a
newer layout when it first starts, and an older binary cannot read that layout.
Putting the old binary back is then not enough. The backup is the way back.

## Upgrade a folder install

Read the release notes of the new version first. Their **What changed**
section lists the changes, and it says so when you have to change something.

Open Settings → Server → **Updates**, choose a stable or beta release, and run
**Prepare**. Preparation verifies the release's Ed25519 signature and checksum,
checks the platform, build profile, disk, active prints and installed modules,
then asks the candidate binary to rehearse migrations on database snapshots.
Apply can run now, when all printers are idle, or at a chosen time.

Immediately before replacement the server drains requests and modules, creates
another verified backup, and atomically keeps the old executable as
`.previous`. The candidate starts as a trial and must become ready. A failed
trial restores both the old executable and its data.

A PostgreSQL database is in the backup only when
`core.upgrade.postgres_backup` is `pg_dump`. With `external` or `none`, apply
asks you to confirm that you protect that database yourself. This holds for
the server's own database and for a module's database. A failed trial then
restores the files and the old executable and finishes as rolled back. It
names each database you have to restore in `external_restore` of the upgrade
record and in a warning in the log. Successful Unix upgrades
replace the process while keeping its PID; Windows starts the final process
after the old executable exits.

The manual download-and-replace procedure below remains the recovery path when
the web application is unavailable:

1. Stop the server with Ctrl+C.
2. Run `prusa-connect backup create`.
3. Download and verify the new binary as
   [Install from a release](./install-from-a-release.md#check-the-download)
   shows.
4. Replace the old file, keep the name, and start the server again.

```text
$ mv prusa_connect_amd64 ~/prusa-connect/prusa_connect_amd64
$ chmod +x ~/prusa-connect/prusa_connect_amd64
$ ~/prusa-connect/prusa_connect_amd64 --version
prusa-connect 0.1.0
```

`--version` prints the version of the file you put there. The first line of
the log and `GET /api/v1/health` name the same version once the server runs.

## Carry an update offline

`go run ./cmd/offline-update-bundle build` makes one signed `.tar` file for a
core executable and optional extension release archives. The archive is
deterministic: it stores `manifest.json`, `manifest.sig`, the core below
`core/`, and extension archives below `extensions/` in sorted order with fixed
metadata. The signed manifest names schema 1, the target platform, build
profile, build variant and core version. It also records the size and SHA-256
digest of each artifact and the identity and compatibility metadata read from
each extension's `module.yaml`.

Use `go run ./cmd/offline-update-bundle inspect -bundle <file> -config
<config.yaml>` before moving the file to its target. Inspection uses the
trusted Ed25519 keys in `core.upgrade.trusted_keys`. Send the file as the
request body of `POST /api/v1/server/upgrade/staged/bundle`. The signed
manifest names the core version. The server rejects unknown keys, changed
bytes, duplicate or undeclared files, unsafe paths, and links. It also rejects
excessive sizes, a different platform or profile, and incompatible extension
metadata before publishing the private staging directory.

Before live data changes, preparation validates each extension archive, its
inner checksum manifest when present, its core range and its wire contract.
`extension_reviews` of the upgrade record lists, for each extension, everything
the release asks for and the requests the installed release did not make. It
also says whether the bundled version is older than the installed one. When a review
names a new request or an older version, apply refuses until you send
`approve_extensions: true`. Apply then grants each extension no more than its
review shows. Apply
then stops the running extensions, takes one backup and installs every archive
before it swaps the core. The candidate trial must serve every bundled
extension and every compatible extension that served before the update. A
failed install, module migration or start restores the previous core, module
directory, overlay, grants and protected module databases from the same
backup. Directory restore removes files that the failed release added.

The static preparation cannot execute an extension migration safely. The
migration runs only in the candidate trial, after the backup exists. A new
extension that needs a setting with no default causes the transaction to roll
back because this bundle format carries no settings. Existing PostgreSQL module
databases require `core.upgrade.postgres_backup: pg_dump`; preparation refuses
the bundle when only external acknowledgement or no PostgreSQL backup is
configured. The server also needs matching `pg_dump`, `pg_restore`, and `psql`
programs. Its restore role needs `CREATEDB` and membership in each database
owner role. A database provisioned by the server uses
`persistence.postgres.module_admin_dsn` for that work. An operator-supplied
database uses the role in its own DSN.

Keep your own `config/config.yaml`. A new release's `config.yaml` can carry new
keys with comments. Compare the two, and copy a key over only when you want it.

On Windows the steps are the same: close the window, replace the `.exe` in
its folder, and start it again.

## Upgrade the systemd service

[Back up the service](#back-up-the-systemd-service) first.

```text
$ sudo systemctl stop prusa-connect
$ sudo install -o prusa-connect -g prusa-connect -m 0755 prusa_connect_amd64 /var/lib/prusa-connect/bin/prusa-connect
$ sudo systemctl start prusa-connect
$ systemctl is-active prusa-connect
active
```

`prusa-connect --version` prints the version you installed.

## Upgrade in Docker

The binary is owned by the immutable image, so the server deliberately never
rewrites it. The Updates page still downloads and verifies the
candidate, runs preflight and migration rehearsal, and creates the backup.
It then reports that Docker manages the executable. After preparation, update
the image.

With the published image, change the version on the `image:` line of your
`compose.yaml` to the new release. Then, in the folder that holds it, download
the image and start the new container:

```bash
docker compose pull
docker compose up -d
```

If you build locally, download the new release's
`prusa-connect-<version>-source.tar.gz` and unpack it beside the old
folder. Copy your `.env` and your `config/config.yaml` from the old folder
into the new one, and write the new version into `.env`. Then build and start
from the new folder:

```bash
docker compose -f docker-compose.local.yml build prusa-connect
docker compose -f docker-compose.local.yml up -d prusa-connect
```

The shipped compose file fixes the name of every volume, such as
`prusa-connect_prusa_connect_data`. So whatever the compose project
is called, the new container finds the same volumes, and with them the same
database and print files.

Kubernetes deployments use the same model. The supplied Deployment runs
`upgrade check` in an init container whenever the host database already exists;
a failed rehearsal prevents the new pod from touching live data. GitOps or the
cluster rollout controller remains responsible for changing the image. Restore
the backup and roll back the image together if the new release cannot start.

## Switch the updates off

A device whose operating system installs every server update itself has no
use for a release the server prepares. Docker and Kubernetes keep the updates
on, because the preparation rehearses a new image, as the sections above
describe. To switch them off, set this in `config.yaml` and restart the
server:

```yaml
core:
  upgrade:
    enabled: false
```

While updates are off, the server prepares no upgrade, keeps no backup of its
own and does not pair with the Beta Portal. It writes no pairing file. The
upgrade, backup and pairing paths under `/api/v1/server/` answer 404, and the
server settings show no **Updates** and no **Backups** tab. The server
version, the restart and the restart in safe mode keep working. The log calls
the updates upgrades, and a start with them off says `upgrades are off`.

Disconnect the Beta Portal pairing on **Updates** before you switch the updates
off. A pairing that is not disconnected keeps its access token, and the portal
accepts the token until the pairing ends. At every start the server names such
a file in its log. To end the pairing then, switch the updates on and
disconnect it. Deleting the file removes only the copy on this server, so
remove the server in the Beta Portal as well.

An upgrade that waits for its scheduled time cannot run while updates are off.
The first start with updates off cancels it, as **Cancel** on **Updates** does,
and says so in its log. Prepare the release again once the updates are on.

The server still uses the `upgrade` folder beside the overlay:

- The `backup` commands put their backups in `upgrade/backups`, unless you name
  another folder with `-to`.
- A start finishes the rollback of an upgrade that an earlier run left
  unfinished.
- A start finishes a restore from a backup that an earlier run asked for and
  left unfinished.

## Back up a folder install

Stop the server, then copy `config` and `storage` into one archive.

On Linux and macOS:

```text
$ (umask 077 && tar -czf prusa-connect-backup.tar.gz -C ~ prusa-connect/config prusa-connect/storage)
$ ls -l prusa-connect-backup.tar.gz
-rw------- 1 you you 25967 Sep 29 21:19 prusa-connect-backup.tar.gz
$ tar -tzf prusa-connect-backup.tar.gz
prusa-connect/config/
prusa-connect/config/config.d/
prusa-connect/config/config.d/authz.db
prusa-connect/config/config.d/resources.json
prusa-connect/config/config.yaml
prusa-connect/storage/
```

On Windows, in PowerShell, in the server's folder:

```text
> Compress-Archive -Path .\config, .\storage -DestinationPath .\prusa-connect-backup.zip
> Get-ChildItem -Name *.zip
prusa-connect-backup.zip
```

`Compress-Archive` leaves out a folder with nothing in it, so an empty
`storage` is missing from the archive. The server creates it again at the
start. The archive takes the rights of the folder it is written in, so write
it in a folder only you can read, such as the server's folder in your profile.

To restore, stop the server, unpack the archive over the server's folder, and
start it again. On Linux and macOS that is
`tar -xzf prusa-connect-backup.tar.gz -C ~`. On Windows, in the server's
folder:

```text
> Expand-Archive .\prusa-connect-backup.zip -DestinationPath . -Force
```

## Back up the systemd service

The archive keeps each file's owner and its full path, so a restore puts every
file back where the service reads it.

```text
$ sudo systemctl stop prusa-connect
$ sudo sh -c 'umask 077 && tar -czf prusa-connect-backup.tar.gz /etc/prusa-connect /var/lib/prusa-connect'
tar: Removing leading `/' from member names
tar: Removing leading `/' from hard link targets
$ ls -l prusa-connect-backup.tar.gz
-rw------- 1 root root 26461 Sep 29 21:13 prusa-connect-backup.tar.gz
$ sudo tar -tzf prusa-connect-backup.tar.gz
etc/prusa-connect/
etc/prusa-connect/config.yaml
var/lib/prusa-connect/
var/lib/prusa-connect/config/
var/lib/prusa-connect/config/config.d/
var/lib/prusa-connect/config/config.d/authz.db
var/lib/prusa-connect/config/config.d/resources.json
var/lib/prusa-connect/storage/
$ sudo systemctl start prusa-connect
```

The two `tar:` lines are normal. To restore:

```text
$ sudo systemctl stop prusa-connect
$ sudo rm -r /var/lib/prusa-connect
$ sudo tar -xzf prusa-connect-backup.tar.gz -C /
$ sudo systemctl start prusa-connect
$ systemctl is-active prusa-connect
active
```

On a new machine, create the `prusa-connect` user before the restore, as
[Run it as a service on Linux](./install-from-a-release.md#run-it-as-a-service-on-linux)
shows. The archive names the owner, and a missing user leaves the files
owned by a number.

## Back up in Docker

The database is in the named volume mounted at `/config/config.d`, and the
print files are in the one mounted at `/storage`. Stop the container, then copy
both into a folder only you can read. `docker cp` reads a stopped container
too:

```bash
docker compose -f docker-compose.local.yml stop prusa-connect
mkdir -m 700 backup
docker cp prusa_connect:/config/config.d ./backup/config.d
docker cp prusa_connect:/storage ./backup/storage
docker compose -f docker-compose.local.yml start prusa-connect
```

With the published image, the container has the same name. Stop and start it
with `docker compose stop` and `docker compose start` in the folder that holds
`compose.yaml`.

The same copy from a test container, which had a name of its own:

```text
$ docker stop pcl-beta-test-core
pcl-beta-test-core
$ mkdir -m 700 backup
$ docker cp pcl-beta-test-core:/config/config.d ./backup/config.d
$ docker cp pcl-beta-test-core:/storage ./backup/storage
$ ls -ld backup
drwx------ 4 you you 4096 Sep 29 21:17 backup
$ ls backup backup/config.d
backup:
config.d
storage

backup/config.d:
authz.db
resources.json
```

`config/config.yaml`, or `config.yaml` beside `compose.yaml`, is already a
file in your folder. Keep a copy of it with the two folders.

## Uninstall a folder install

Stop the server and delete its folder. That removes the binary, the
configuration, the database and the print files. Make a backup first if you
want any of them.

## Uninstall the systemd service

```text
$ sudo systemctl disable --now prusa-connect
Removed /etc/systemd/system/multi-user.target.wants/prusa-connect.service.
$ sudo rm /etc/systemd/system/prusa-connect.service /usr/local/bin/prusa-connect
$ sudo systemctl daemon-reload
$ sudo rm -r /etc/prusa-connect /var/lib/prusa-connect
$ sudo userdel prusa-connect
$ systemctl status prusa-connect --no-pager; echo exit=$?
Unit prusa-connect.service could not be found.
exit=4
```

The fourth line deletes the configuration, the database and the print files.
Leave it out to keep them.

## Uninstall in Docker

In the folder that holds `compose.yaml`:

```bash
docker compose down -v
```

In the folder you built from:

```bash
docker compose -f docker-compose.local.yml down -v
```

`-v` deletes the named volumes, which hold the database and your uploads.
Without `-v` the volumes stay. The image stays too.
`docker image rm ghcr.io/prusa3d/prusa-connect:<version>` deletes the
published one, and `docker image rm prusa-connect:local` the one you built.

## Stop a printer from connecting

A printer that joined keeps trying to connect after the server is gone. On the
printer, open **Settings**, **Network**, **Prusa Connect**, and set **Enabled**
to **OFF**.
