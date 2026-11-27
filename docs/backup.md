# Backup and Restore

## Create a backup

Run a one-time backup with:

```sh
redxt --maintenance backup
```

Supply `--include-ssl` to include the configured SSL directory and
`--include-data` to include the data directory. A custom filename and
`--password` are also accepted. Without a password, the archive is unencrypted
unless compliance mode blocks the operation. Scheduled backups use the saved
backup password and do not prompt.

The archive includes `server.yml`, local `server.db` and `users.db` files,
`app_secrets.json`, `secrets.yml`, `yaml.d/`, `assets/`, and configured template and theme files
when present. The copy of `server.token` is blanked. Database files are
snapshotted with SQLite `VACUUM INTO`; `app_secrets.json` is exported from the
same server database snapshot and restores the allowlisted cryptographic keys,
including available previous versions and their expiry timestamps. Referenced database and cache TLS files
are always included. Manually configured certificate and key files outside
the SSL directory are stored under `ssl-external/`; restore moves them into
the target SSL directory and rewrites `server.ssl.cert` and `server.ssl.key`.
The SSL directory's unrelated files are included only with `--include-ssl`.

## Scheduled backups

The first daily run creates a full backup. Later runs create a cumulative
incremental from the previous full to the current snapshot, verify it, then
create the full backup for that same snapshot. Incrementals contain changed
files and a list of removed paths; changed SQLite databases are stored whole.
The hourly task creates an incremental from the newest full and requires that
a full backup already exists.

Both full and incremental archives carry a manifest with file sizes, Unix
modification times, SHA-256 hashes, and the snapshot time. Encrypted archives
use Argon2id and AES-256-GCM. Their authenticated `BKUP1` header includes the
key identifier and, for incrementals, the base filename.

## Restore

Restore a full archive or an incremental archive with:

```sh
redxt --maintenance restore <backup-file>
```

Encrypted archives require `--password` or an interactive password prompt.
An incremental restore requires its named full base in the same directory;
the base hash is checked before any restore writes begin. The restore applies
the changed files over the base and then removes the paths listed in the
incremental manifest.

Archive verification and restore enforce `server.restore` limits before and
during extraction. Defaults are a 1 GiB archive, 4 GiB expanded content,
10,000 files, 512 MiB per file, and a 100:1 expansion ratio. Incremental
restores apply the expanded-size, file-count, total-archive, and ratio limits
to the base and incremental together. Extracted file permissions are
normalized; archive executable bits are not preserved.

The target's operator token is kept when one exists. Otherwise restore creates
a new operator token and prints it once. A restore onto an empty database also
prints a one-time setup token and stores only its SHA-256 hash in the restored
server database. The CLI token is prepared for the new-server Primary Admin
re-authentication flow. After the restored server starts, open
`/server/administration/setup?reauth=1` (replace `administration` if
`server.admin_path` is customized), enter the token, and continue. The token
creates a session for the preserved Primary Admin, leaves its credentials and
settings intact, and cannot be reused.

## Retention

Full backups are assigned to one exclusive retention bucket: January 1st,
other first-of-month dates, Sundays, then all remaining dates. Incrementals
are excluded from count limits and replaced by later runs. A retained
incremental's base full is protected so the pair remains restorable. The size
cap considers all backup files; if it cannot be met without deleting a
protected base, the cap remains exceeded and a warning is logged.

The authenticated admin panel at `/server/{admin_path}/backup` can create a
manual backup, list archives, verify an archive, delete an archive, and
download an archive as a no-store attachment. It refuses to delete a full
backup that a retained incremental needs. Restore and backup policy editing
are not yet exposed in the panel; use the maintenance restore command for
restores.
