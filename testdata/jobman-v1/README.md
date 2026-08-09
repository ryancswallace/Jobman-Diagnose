# Copied Jobman evidence fixtures

The JSON values in this directory are immutable copies of Jobman's diagnostic
evidence schema 1 compatibility fixtures. `manifest.json` records the source
Jobman v1.4.0 release, schema, semantic evidence IDs, and exact file SHA-256
values.

Companion tests consume these files without accessing the network or a local
Jobman checkout. Update the copies only when adopting an intentional, published
core fixture set, and retain older supported fixture sets in separate
directories.
