### Fixed
- A backup made while no update was waiting was named as if it led to a version called `unknown`, such as `20261123T104715Z-0.1.0-beta.1-to-unknown`. It is now named after the date and the version that made it, such as `20261123T104715Z-0.1.0-beta.1`, and the Backups page and `prusa-connect backup list` show only that version.
