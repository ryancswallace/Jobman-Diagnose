# Deployment configuration

Organization-specific values, approved models, disclosure selections, notices,
and operational documentation belong in an external deployment repository.
Diagnose publishes a neutral native configuration contract and never reads a Git
checkout automatically. Secret values, evidence, reports, and generated runtime
state do not belong in configuration Git repositories.

## Selection and validation

Configuration selection is explicit `--diagnosis-config PATH`, then
`JOBMAN_DIAGNOSE_CONFIG`, then the existing per-user `jobman/diagnosis.yml`.
Deployments may opt into a system fallback by setting
`JOBMAN_DIAGNOSE_SYSTEM_CONFIG` to a clean absolute path. This fallback is used
only when the default user file does not exist. An unreadable, malformed, or
indirect user file fails; it does not silently select the system file. Explicit
flag and environment selections likewise never fall back on failure. There is
no layer merge, implicit project discovery, or shell expansion.

```console
jobman-diagnose config paths
jobman-diagnose config schema
jobman-diagnose config validate /absolute/path/diagnosis.yml
jobman-diagnose config show /absolute/path/diagnosis.yml
```

`schema` prints JSON Schema draft 2020-12 field shapes for the complete native
schema 2, including new optional transport fields. `validate` remains the
authoritative check for cross-field provider, locality, source, and disclosure
constraints. It reads only the selected configuration file and never resolves
credentials, CA files, client keys, or endpoints. `show` emits references, not
resolved values; references may still contain private deployment metadata.

Deterministic analysis remains independent of this configuration. Selecting a
default profile does not enable generation. `--ai` or an explicit profile is
still required, and disclosure remains the intersection of invocation approval
and the profile's hard limits. A system profile is an overridable default, not
an enforced organization security policy.

## Enterprise HTTPS transport

HTTP profiles may contain an optional `transport` mapping:

```yaml
transport:
  ca_file: /etc/jobman/model-service-ca.pem
  client_certificate_file: /run/credentials/jobman-diagnose/client.pem
  client_key_file: /run/credentials/jobman-diagnose/client-key.pem
  proxy_url: http://proxy.example.invalid:3128
```

Each field is optional. Certificate and key must appear together. All file
references must be clean absolute paths. Configured transport requires an HTTPS
endpoint. `ca_file` selects an exclusive trusted CA bundle; omitted means system
trust. Hostname verification and TLS 1.2 minimum remain enforced, and there is
no skip-verification switch. Client keys must be private regular files; CA and
certificate files must not be group/world writable. Files are bounded to 1 MiB,
and symlink inputs are rejected. References are read only when constructing an
explicitly requested generator.

Proxy URLs are explicit HTTP(S) origins without credentials, paths, query, or
fragments. Ambient proxy environment variables remain ignored. A remote HTTPS
provider uses CONNECT through an HTTP proxy. Local profiles may never use a
proxy, and every local dial address must still resolve to loopback. A model
service on another internal host is therefore `remote`, even on a private
network. Redirects remain rejected. Command providers reject HTTP transport
configuration entirely; they retain their existing isolated environment.

An HTTPS proxy uses the transport trust policy too, so a selected CA bundle
must trust both required endpoints. Combining an HTTPS proxy with a client
certificate is rejected because the underlying transport could present that
certificate to the proxy. Use an HTTP CONNECT proxy with the HTTPS provider
endpoint when provider mTLS is required. Authenticated proxy configuration and
arbitrary custom headers are intentionally not provided. Approving a transport
does not approve additional evidence disclosure or enable a provider.

## Defaults and scope

No provider, endpoint, model, credential, proxy, or custom trust root is selected
by default. Existing profile timeout/input/output/disclosure bounds are unchanged.
Deployments should start with deterministic-only operation; a later approved
generation profile can allow only bounded metadata, with source mode `none`.
Command/path/environment-name disclosure must be excluded explicitly from a
metadata-only profile because invocation options can otherwise approve them.

Native inventory families are `schema_version`, `defaults.profile`, and named
`profiles` containing provider/locality, endpoint or command, model, required
structured-output capability, time/byte bounds, per-class disclosure bounds,
source context policy, credential reference, and transport references. The schema
command derives field names from native types so external tooling does not need
a separate handwritten field list.

Provider prompts, taxonomies, schemas, citation validation, fixed action
identifiers, redaction boundaries, and the read-only Diagnose contract remain
product code. Model credentials never enter Core Jobman or Dashboard. Store
system profiles and references outside source checkouts; install them with
restricted permissions and pin the native validator version used by deployment
composition. Configuration changes take effect on the next invocation.
