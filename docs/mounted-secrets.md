# Mounted secrets (DEVOPS-645)

The application loads every regular file from `/opt/secrets` into its existing
environment before reading configuration. The deployment chart controls whether
this mount exists. Without it, startup behavior is unchanged.

- An existing environment variable wins, including an explicitly empty value.
- Files one directory below the root load before top-level static KV files.
- Entries are sorted; first writer wins. Hidden entries and deeper directories are skipped.
- Unreadable dynamic keys remain claimed, so stale static values cannot replace them.
- Directory discovery completes before loading any files. If a directory cannot be
  inspected/listed, no mounted values load; existing environment values remain intact.
- Filesystem failures warn without aborting bootstrap. Warnings contain names and paths,
  never values or exception messages that could contain values.
- Values are trimmed. Whitespace-sensitive values require review before cutover.
- Kubernetes projected-volume symlinks are supported.

Application config validation remains responsible for missing required credentials.
The loader runs once at startup; credential rotation requires a new process (a new
CronJob run or a deployment restart/Reloader).

Rollout order: test and build this application branch, verify required Vault keys,
then enable `vaultSecrets.mountSecrets` in sre-deployments with the tested image.
Remove conflicting secret environment entries during that cutover. Keep this app
PR open for its owning team; deployment rollouts may use the tested branch image.
Rollback requires restoring both the previous image and its secret-delivery mode.
