<!-- cspell:ignore seqs -->

# Live-provider promotion evidence

This record completes the live-provider gate for Jobman Diagnose v0.7.1. The
evaluated production source is commit
`ddde479862f7b9991d894e98f7c16b9c2e63f571`. The stable tag is a
documentation-only descendant; no production source changed after the live
gates.

The corpus and promotion policy are the checked-in schema-4, 72-case corpus
and schema-1 policy. Each promotion run executed every case three times, for
216 provider-required executions and 144 repeated-result comparisons. Provider
fallback was disabled. Source context used the limited mode for all 29 mapped
cases, producing 87 source-context executions.

## Evaluated providers

| Property | Hosted strict-output service | Local runtime |
| --- | --- | --- |
| Profile | `hosted-openai` | `local-vllm` |
| Adapter and locality | `openai_compatible`, remote | `openai_compatible`, local loopback |
| Model | `gpt-6-luna` | `mlx-community/Qwen3-30B-A3B-Instruct-2507-6bit`, snapshot `f5ace4e14ed07217d0f3628ece83ca368f19ee70`, served as `qwen3-30b-a3b-instruct-2507-6bit` |
| Runtime | OpenAI Chat Completions strict structured output, service-managed | vLLM Metal 0.33.0 with vLLM core 0.33.0+cpu, Guidance structured-output backend, macOS 26.6.2 arm64 |
| Doctor | Ready; all eight checks passed | Ready; all eight checks passed |
| Promotion | 216/216; policy passed | 216/216; policy passed |

Both profiles allowed at most 524,288 input bytes and 32,768 output bytes, two
analysis passes, 256 metadata items/131,072 bytes, 16 command items/131,072
bytes, 256 path items/131,072 bytes, 256 environment-name items/131,072 bytes,
four log artifacts/65,536 bytes, and six source artifacts/65,536 bytes. Both
used limited source context with 60 lines on either side. The hosted timeout
was two minutes with reasoning effort `none`; the local timeout was five
minutes.

The local server used this bounded launch configuration:

```console
vllm serve mlx-community/Qwen3-30B-A3B-Instruct-2507-6bit \
  --host 127.0.0.1 \
  --port 8001 \
  --served-model-name qwen3-30b-a3b-instruct-2507-6bit \
  --max-model-len 16384 \
  --max-num-seqs 1 \
  --max-num-batched-tokens 16384 \
  --generation-config vllm \
  --structured-outputs-config.backend guidance \
  --gpu-memory-utilization 0.85
```

vLLM's default XGrammar backend was not used because vLLM 0.33.0 failed to
compile the request-specific proposal schema. Guidance compiled the same strict
schema, and the provider doctor confirmed the complete request and response
contract before evaluation.

## Promotion summaries

| Metric | Hosted | Local |
| --- | ---: | ---: |
| Primary-code precision | 1.000000 | 1.000000 |
| Unsupported-claim rate | 0.000000 | 0.000000 |
| Citation validity | 1.000000 | 1.000000 |
| Safe-action rate | 1.000000 | 1.000000 |
| Retry-advice accuracy | 1.000000 | 1.000000 |
| Deterministic stability | 1.000000 | 1.000000 |
| Provider-fallback rate | 0.000000 | 0.000000 |
| Proposal acceptance | 1.000000 | 1.000000 |
| Useful-diagnosis rate | 1.000000 | 1.000000 |
| Taxonomy accuracy | 1.000000 | 1.000000 |
| Entity preservation | 1.000000 | 1.000000 |
| Causal completeness | 1.000000 | 1.000000 |
| Abstention accuracy | 1.000000 | 1.000000 |
| Forbidden-claim rate | 0.000000 | 0.000000 |
| Citation economy | 1.000000 | 1.000000 |
| Generated consistency | 0.986111 | 0.979167 |

Each result recorded 87 source-context executions and three expected mismatch
detections: every repetition of the `source_log_disagreement` control. All six
executions passed, confirming that stale source was identified rather than
treated as current causal authority.

## Safety controls

The live gate repeated the `secret_canary` and `prompt_injection_noise`
controls three times per provider. It also exercised timeout and connection-
refusal cases, required explicit abstention for insufficient evidence, and
required zero provider fallback, unsupported claims, and forbidden claims.

The same candidate passed the repository's release checks for response secret
canaries, provider refusal, malformed and oversized structured output, request
timeout and cancellation, redirect and locality enforcement, semantic
rejection, and deterministic fallback. These controls remain deterministic
tests because a live provider cannot be instructed to reproduce transport and
protocol failures reliably.

## Retained artifacts

Raw proposal captures contain complete model output and disclosed synthetic
fixture content. They are retained privately with user-only permissions and
are intentionally excluded from Git, as required by the evaluation contract.
The public record retains their SHA-256 identities and the complete non-secret
summaries:

| Artifact | SHA-256 |
| --- | --- |
| Hosted promotion result | `ed9d01ce354b38a512748e6557af0b6f17d491271791055a72fc22f3e1c67449` |
| Hosted private proposal capture | `9b6cb47bb5d3b7d8bf5143c8bb2bbf016ac9f322ef4429dbfcf728f314de3a44` |
| Local promotion result | `db089d329d074fa775bf5929b84bd9431a68a588ce8c73100d9362a268ba0bc7` |
| Local private proposal capture | `ae28de9dde10b8110a74400a8f830be72803828025af8b9599407f80b3314e55` |

The immutable candidate identities `v0.7.0-rc.1` through `v0.7.0-rc.5` and
stable identity `v0.7.0` remain unchanged. The `v0.7.1` tag records the
validated patch release; none of these identities is moved or redefined. This
evidence records validation and does not independently approve a deployment.
