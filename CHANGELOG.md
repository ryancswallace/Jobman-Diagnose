# Changelog

All notable user-visible changes to `jobman-diagnose` are documented here. The
format follows [Keep a Changelog], and releases use [Semantic Versioning].

## [Unreleased]

## [0.6.0] - 2026-08-13

### Added

- Added `jobman diagnose doctor` with human and schema-1 JSON output to resolve
  a selected credential, validate adapter capabilities, and exercise the real
  provider/model through a fixed evidence-free structured causal probe.

- Expanded the release corpus from 60 to 72 cases with sanitized
  real-world-style timestamped, ANSI-colored, long, noisy, interleaved,
  multi-cause, JVM, Go, Node.js, Rust, shell, build, and container-control
  incidents, plus a stale-source disagreement control and 29 total source
  mappings.
- Added a checked-in live-evaluation promotion policy and `make
  evaluate-release` gate requiring the complete representative corpus, three
  repetitions, source-context coverage, provider invocation and consistency
  denominators, zero fallback and unsupported claims, and explicit quality
  floors. Evaluation-result schema 6 embeds the policy assessment plus
  per-case and aggregate stale-source detection.

### Changed

- Replaced whole-tail provider projection with deterministic causal-context
  selection that ranks exact enrichment across runs and streams, selects
  continuous bounded windows with byte/line/digest provenance, rebases
  enrichment ranges, and falls back to terminal output only when needed.
- Expanded implicitly sized live log collection to a 1 MiB local search bound
  when log disclosure is explicit; `--log-bytes` remains authoritative and
  provider disclosure stays within the profile ceiling.
- Upgraded generation requests to schema 5 for provenance-carrying log-context
  selections and selected-content integrity validation.
- Compared current source context with source locations in target output,
  reported detectable file, line-range, and Python traceback-line mismatches,
  and withheld mismatched source from provider projection. Matching locations
  remain point-in-time context rather than executed-revision attestation.

- Classified exact enriched diagnostic lines into controlled address,
  authentication, configuration, connection, data, database, deadline,
  dependency, DNS, resource, linker, migration, filesystem, rate-limit,
  service, storage, and TLS subtypes.
- Promoted those subtypes above a generic nonzero exit as specific
  deterministic `target.*` diagnoses, with class-specific read-only
  inspection actions and persistent-versus-transient retry advice. When logs
  contain several recognized signals, the final diagnostic line is primary
  and earlier signals remain secondary findings.
- Accepted application-configuration taxonomy for explicit missing environment
  values, stopped requiring an incidental retry delay in the HTTP 429 case,
  and stopped requiring a Rust stack-frame operation after the rejected record
  and invalid value already make the diagnosis actionable.

### Fixed

- Fixed generated network grounding for timestamped and source-annotated log
  lines by checking every plausible endpoint, excluding timestamp fragments
  and source filenames, and removing trailing diagnostic punctuation before
  comparing the model's retained endpoint.
- Corrected the release policy's citation-economy denominator so the threshold
  is attainable by the corpus's 165 repeated required-cause executions.
- Corrected source-context promotion thresholds to cover all 29 mapped cases
  across three repetitions and fixed mismatched versions in the published
  artifact verification example.

### Security

- Updated the pinned Go toolchain from 1.26.5 to 1.26.6 to incorporate fixes
  for four reachable standard-library vulnerabilities reported by
  `govulncheck`.
- Prevented detectably stale or mismatched current source from entering model
  context while preserving a controlled report warning for operator review.
- Kept specific target-log diagnoses explicitly framed as target-reported
  conditions rather than independently confirmed host, network, database, or
  filesystem facts; ambiguous kill and crash-loop messages continue to
  abstain from invented causes.

## [0.5.0] - 2026-08-13

### Changed

- Focused structured generation with a shorter evidence-first instruction
  contract and richer bounded diagnostic lines that preserve multi-traceback
  chains, exception-group branches, validation details, syntax locations,
  operations, and remediation messages.
- Made explicit HTTP 5xx responses authorize
  `generated.external_service_failure` without also authorizing generic
  reachability or transience classifications.
- Aligned generated-quality evaluation expectations with the diagnosis
  contract by accepting equivalent causal phrasing, excluding incidental
  remediation, lifecycle, and output details, and normalizing formatting-only
  diagnostic identifier differences.
- Added profile-level `source_context` defaults and
  `--ai-source none|limited|full` overrides for disabled, symmetric limited,
  or full-file source sharing, including configurable lines on each side of
  the failure anchor.
- Extended live corpus evaluation to honor profile source defaults, attach 28
  checked-in source mappings while still running all 60 cases, and report
  source use per result and in aggregate.
- Made development evaluation results and proposal captures atomically replace
  existing private files so repeated runs can reuse stable output paths.
- Added source-code context with exact
  line/byte provenance, direct-command inference, `--source-file` and
  `--source-line` controls, strict profile limits, provider projection,
  citations, human disclosure, and support-bundle retention.
- Upgraded generation requests to schema 4 so request identity commits to the
  complete companion analysis evidence, including attributed enrichment and
  any explicitly selected source snapshot.
- Upgraded support bundles to schema 2 and replaced `enrichment.json` with
  `analysis-context.json`, which retains enrichment and explicitly selected
  source context together.
- Focused AI augmentation on one evidence-backed root cause, removed routine
  resource-usage observations and store-local fingerprints from model context,
  and refined traceback, dependency, permission, configuration, network, and
  resource classification guidance for smaller local models.
- Prevented schema-description echoes, Jobman lifecycle narration, and
  evidence-unsupported secondary cause phrases from appearing as generated
  failure paths while retaining a separately supported root cause.
- Made the Python failure-lab runner prefer the companion built in this
  checkout and report the exact Jobman and Jobman Diagnose executables used.
- Added private live-evaluation proposal capture with host acceptance reasons
  and automatic metadata-only routing for fixtures that do not advertise the
  configured-value redaction capability required for log disclosure.
- Upgraded generation requests to schema 3 with bounded diagnostic lines
  deterministically selected from already disclosed traceback/compiler ranges,
  making inner exception causes and operations explicit without expanding log
  disclosure.
- Expanded the generated evaluation corpus to 60 multi-ecosystem cases with
  required facts and causal relations, forbidden claims, abstention controls,
  language/failure/format tags, focused selection, bounded repetition, and
  proposal captures labeled by case and iteration.
- Added usefulness, taxonomy, entity-preservation, causal-completeness,
  abstention, forbidden-claim, citation-economy, acceptance, and repeated-run
  consistency metrics without collapsing them into one quality score.
- Added an executable shell, Go, Node.js, C, JVM, and Rust failure lab that
  prefers the freshly built companion and skips unavailable optional
  toolchains explicitly.

### Fixed

- Corrected the compatibility and command-bridge documentation to identify
  evaluation corpus/result schema 4, generation request schema 4, and command
  protocol 3, and made documentation validation reject contract-version drift
  from code constants.
- Corrected the pipeline cause-chain evaluation traceback to reference the
  actual outer source line so limited source collection can use its runtime
  anchor.
- Made persisted source-context and generation-request paths validate
  consistently when evidence is moved between POSIX and Windows systems.

### Security

- Kept current source text outside core execution truth: it is unredacted,
  point-in-time supplemental context, cannot independently authorize a
  generated cause, and is rejected when ambiguous, non-regular, symlinked,
  unstable, invalid UTF-8, NUL-bearing, or over its hard/profile byte limits.
- Added host-side causal-class validation that rejects generated claims when
  the model's own citations do not contain a corresponding direct signal. When
  no projected artifact supports any generated cause, the strict schema forces
  the model to abstain and Jobman returns the deterministic diagnosis.
- Added exact-signal preservation for endpoints, TLS, DNS, database, linker,
  panic, and related causal messages, and schema-enforced abstention when a log
  declares that it was truncated before the terminal cause.

## [0.4.0] - 2026-08-11

### Changed

- Upgraded generated proposals to schema 2 with distinct issue summary, root
  cause, and failure-path fields; expanded the cause taxonomy and prompts so
  AI-assisted diagnoses identify the specific exception, setting, dependency,
  resource, operation, path, endpoint, or rejected value instead of restating
  the nonzero exit mechanism.
- Added host-side rejection of repeated, generic, and evidence-plumbing
  diagnoses plus generated-specificity evaluation expectations for log-backed
  compiler, Go, JVM, Python, permission, dependency, network, and storage
  failures.

### Security

- Allowed short diagnostic fragments from explicitly approved, bounded log
  projections to enter advisory generated prose when needed for specificity;
  complete artifacts, commands, URLs, tools, retry authority, and mutations
  remain prohibited, and generated text remains untrusted.

## [0.3.0] - 2026-08-10

### Added

- Added a standard-library Python failure lab with simple, layered, timeout,
  signal, and concurrent failures plus a runner that submits each fixture and
  displays its AI-assisted diagnosis.
- Added reproducible terminal-demo sources and GIF, MP4, and WebM outputs.

### Fixed

- Specialized every AI response schema with the exact request identity and
  request-specific code, category, evidence, finding, and action catalogs so
  grammar-constrained local models cannot emit structurally valid but
  unauthorized scalar values. Generation requests and command bridges now use
  protocol 2, whose derived-schema verification avoids a request-hash cycle
  without weakening host-side semantic validation. Fixed relational guidance
  also prevents grammar-constrained local models from cross-listing the same
  citation where portable JSON Schema cannot express that relationship.

### Changed

- Reworked human diagnosis output into an answer-first summary that places
  validated AI-assisted causes beside Jobman's confirmed finding, prioritizes
  recommendations and retry guidance, shows a small relevant evidence set,
  and moves the complete evidence and provenance audit to `--details`.
- Added a consistent bullet and indentation hierarchy plus semantic terminal
  color controlled by `--color=auto|always|never`. Automatic color is
  terminal-aware, honors `NO_COLOR` and `TERM=dumb`, and never affects JSON.
- Added specific hypothesis taxonomy guidance, minimal-citation guidance, and
  trusted non-executing recommendations authored by Jobman for supported
  generated cause codes. Generated commands, URLs, and arbitrary actions
  remain prohibited.
- Documented installing Jobman and Jobman Diagnose together from Cloudsmith's
  Debian, RPM, and Alpine repositories beginning with Jobman v1.5.0.

## [0.2.0] - 2026-08-09

### Added

- Added Homebrew distribution through `ryancswallace/homebrew-tap`, generated
  only from the checksums of an already-public stable GitHub release.
- Added `.deb`, `.rpm`, and `.apk` packages for Linux 386, amd64, and arm64,
  per-package SPDX SBOMs, release checks, and idempotent publication to the
  public Cloudsmith repository.
- Added separately repairable post-release workflows with protected
  credentials, checksum-signature and attestation verification, and
  source-digest tags that detect conflicting Cloudsmith packages.
- Added assembled compatibility tests against Jobman v1.4.0 and current
  Jobman `main`, including direct invocation, extension dispatch, evidence
  export/import, private permissions, and semantic report parity.
- Added ShellCheck, release-metadata consistency checks, scheduled invariant
  validation, verified retained-draft publication, and DEB/RPM/APK install
  smoke tests in pinned target-distribution containers.
- Expanded the synthetic evaluation corpus with reproducible Python, Go, JVM,
  compiler, launch, network, storage, and adversarial-log cases, plus generator
  drift and ordinary CI quality gates.
- Added optional system-context acquisition and automatic AI-mode collection
  of bounded state-filesystem and Linux cgroup-v2 constraints, with readable
  citations and compatibility fallback for Jobman v1.4.0.

### Changed

- Changed Homebrew publication to open an automatically merged, required-check
  pull request in the protected shared tap instead of pushing directly to its
  default branch.

### Fixed

- Explicitly dispatch post-release distribution workflows after a staged
  release is published with `GITHUB_TOKEN`, whose ordinary release event does
  not start additional workflows.
- Made pinned tool bootstrap recipes fail immediately instead of risking a
  success marker after a failed installation.
- Accepted GitHub's normal `targetCommitish: main` metadata during retained
  draft recovery while retaining immutable tag, signature, attestation, and
  executable-version checks.
- Serialized native-package generation so release validation cannot retain
  incomplete output from concurrent nFPM packaging.
- Distinguished cumulative shared-cgroup OOM counters from per-run facts so
  generated analysis can use them as context without treating them as proof.

## [0.1.0] - 2026-08-09

### Added

- Added bounded live evidence acquisition through `jobman show evidence` and
  offline import of raw evidence or the core CLI envelope.
- Added deterministic diagnoses for direct launch failures, timeouts,
  cancellation, ownership loss, signals, nonzero exits, log degradation,
  notification failures, repeated failures, and conservative target-log
  signatures.
- Added sealed diagnosis report schema 1 with controlled findings, confidence,
  citations, actions, retry advice, limitations, provenance, and disclosure.
- Added human and JSON output plus private, atomic, no-overwrite evidence and
  report exports.
- Added evidence-aware human output with compact finding/citation aliases,
  readable policy text, typed fact values, wrapped sections, suggested command
  rendering, and complete technical provenance without raw log content.
- Added explicit generated augmentation with strict schema-2 profile configuration,
  per-class CLI disclosure approval, exact projection manifests, bounded
  request/proposal protocols, semantic citation validation, contradiction
  handling, and deterministic fallback.
- Added an absolute local command bridge, a strict OpenAI-compatible Chat
  Completions adapter, and a loopback-only Ollama adapter without adding a
  provider SDK.
- Added environment/private-file credential references, transport locality
  enforcement, proxy-free and redirect-free HTTP, and `--require-model`.
- Added copied Jobman evidence schema 1 fixtures and offline compatibility
  tests, including a maximum-budget and secret-canary case.
- Added deterministic use of typed process resource observations and exact
  store-local failure-fingerprint history, including cited recurrence
  findings, partial/truncated-history warnings, and history-aware retry advice.
- Added pinned standalone development tools, vulnerability and workflow checks,
  native race CI, supported-architecture builds, and a GoReleaser archive,
  checksum, and SBOM configuration.
- Added per-user diagnosis configuration discovery, a configured default
  profile, `--ai`/`-a`, `--profile`, and `--ai-logs`, with metadata implied by
  AI activation and live log-tail collection implied by log-content approval.
- Added delayed interactive AI progress with elapsed phase time, provider
  timeout and cancellation context, plus `--progress auto|plain|off`; automatic
  JSON and redirected output remain silent.
- Added `config paths`, `config validate`, `config show`, and `profiles`
  inspection commands that never resolve credentials or invoke a provider.
- Added typed execution-context disclosure for direct executable/argument
  vectors, filesystem paths, resolved executables, and value-free environment
  names, enabled by AI activation, plus an exact deterministic diagnosis for
  the standard `false` utility.
- Added a sealed analysis-evidence wrapper with deterministic exact-range
  Python, Go, JVM, and compiler enrichment; reports expose collector/analyzer
  provenance and models receive enrichment only with its approved source log.
- Added current-policy-aware retry reasons, stable diagnosis grouping
  fingerprints, generator descriptors, and allowlisted read-only Jobman
  argument-vector actions.
- Added deterministic private support bundles with dry-run inventories,
  per-member hashes, capability/build metadata, and no-overwrite creation.
- Added a checked-in evaluation corpus and deterministic/live runner with
  separate correctness and safety metrics, plus decoder fuzz targets and
  scheduled CodeQL/fuzz workflows.
- Restricted generated findings to a host-owned hypothesis taxonomy included
  in every sealed request.
- Added keyless checksum signing, SBOM packaging, build metadata, and an
  attested draft-release workflow that rejects development module replacements.
- Added contributor, support, conduct, citation, issue, pull-request, and
  third-party notice metadata plus a navigable installation and troubleshooting
  documentation set.
- Added dependency review, OpenSSF Scorecard, Dependabot, synchronized labels,
  scheduled external-link checks, repository settings-as-code, and a pinned
  multi-architecture devcontainer.
- Added an aggregate coverage floor and deterministic repository-relative
  documentation link validation plus pinned spelling checks to local and
  continuous-integration gates.
- Hardened the draft-release gate with protected environment approval,
  semantic-tag and main-branch verification, full source revalidation, and a
  tagged Jobman dependency requirement.
- Established Jobman v1.4.0 as the minimum tagged core dependency and made
  source builds and continuous integration independent of a sibling checkout.

### Fixed

- Honor Windows file-mode semantics for exports, credentials, and executable
  validation, and preserve byte-exact evidence fixtures across Windows checkouts.
- Distinguish terminal completion reasons such as `failure_limit` from an
  active job that is waiting for prerequisites when summarizing retry policy.
- Made `--help` a successful informational invocation instead of returning the
  command-usage exit status.
- Honor each generation profile's configured timeout while waiting for HTTP
  response headers, allowing non-streaming local inference to run beyond the
  previous fixed 30-second transport limit.
- Report stable, nonsecret provider failure classifications in required-model
  errors and deterministic fallback warnings without exposing response bodies,
  generated content, evidence, or credentials.
- Use greedy generation and concise proposal guidance for OpenAI-compatible
  providers, reducing long or incomplete outputs from smaller local models.
- Suppress generated findings that merely duplicate an existing deterministic
  finding with the same citations.

[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
[Unreleased]: https://github.com/ryancswallace/Jobman-Diagnose/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.6.0
[0.5.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.5.0
[0.4.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.4.0
[0.3.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.3.0
[0.2.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.2.0
[0.1.0]: https://github.com/ryancswallace/Jobman-Diagnose/releases/tag/v0.1.0
