---
title: Safety model
updated: 2026-07-15
status: implemented
---

# Safety

When state is uncertain, the extension avoids overwriting content and preserves
information the user can inspect. Guarantees differ by operation.

- install: never overwrite a destination with unknown ownership
- publish: add only unmanaged skills to empty remote paths; adopt identical
  existing content and reject different content
- pull: stage new content, reread local content before replacement, and restore
  the original directory if the manifest update fails
- push: recheck the source skill tree SHA after cloning and use only a normal
  push
- proposal: use one generated same-repository branch per skill, reject unknown
  ownership or external changes, and never force-push or rebase
- uninstall: preserve local changes unless `--force` explicitly discards
  differences from the synchronization point
- merge: leave markers for text conflicts and stop before workspace changes for
  binary or structural conflicts
- credentials: never save credentials in repository configuration and redact
  them from logs

The extension transfers raw bytes and executable state. The system umask still
affects the mode of new files. If the manifest update fails after a successful
push or publish, the remote change is not rolled back.

There is no process lock, journal, or signal recovery. See
[Transactions](../storage/transactions.md).
