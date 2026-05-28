# OpenAI Image Provider — Transition Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add OpenAI's GPT Image API (gpt-image-2 / 1.5 / 1-mini) as a first-class provider in imagemage, switch the default to OpenAI, and demote Gemini to an opt-in `--provider=gemini` legacy mode.

**Architecture:** Extract a small provider interface (`pkg/provider`) with two implementations (`pkg/openai`, existing `pkg/gemini`). Commands depend on the interface, not on Gemini directly. A `--provider` flag plus `IMAGEMAGE_PROVIDER` env var selects the backend; per-feature capability differences are handled by the provider returning `ErrUnsupported` so commands can fall back, error, or warn deterministically.

**Tech Stack:** Go 1.25, Cobra, `net/http`, `mime/multipart` for OpenAI's `/v1/images/edits` endpoint, no new external deps.

---

## Research Summary (May 2026)

### OpenAI image model lineup

| Model | Released | Notes | Per-image price (low → high quality) |
|---|---|---|---|
| `gpt-image-1` | Mar 2025 | Original autoregressive image model | $0.011 → $0.167 |
| `gpt-image-1-mini` | Oct 2025 | 80% cheaper than gpt-image-1 | $0.005 → $0.036 |
| `gpt-image-1.5` | Dec 2025 | 4× faster, 20% cheaper, fixed cropping/warm-color bias, `input_fidelity=high` for edits | $0.009 → ~$0.17 |
| `gpt-image-2` | Apr 2026 | Adds reasoning into generation; current quality leader | not yet published in tracked sources |

### API surface

- Generation: `POST https://api.openai.com/v1/images/generations` — JSON body, `Authorization: Bearer $OPENAI_API_KEY`.
- Editing/composition: `POST https://api.openai.com/v1/images/edits` — **multipart/form-data**, accepts up to 16 input images.
- Sizes are fixed: `1024x1024`, `1024x1536`, `1536x1024` (no 4K; no 21:9; no `--frugal` ladder).
- Response is base64 (`data[i].b64_json`). gpt-image-* does **not** support `response_format=url`, only the b64 form, which maps cleanly to imagemage's existing pipeline.
- Quality tiers: `low` / `medium` / `high`.
- Server-side batching: `n` up to 10 images per request.
- Verification: OpenAI requires "API Organization Verification" before any gpt-image-* model is callable.

### Quality vs Gemini

Per the LM Arena snapshot in tracked sources: GPT Image 1.5 ≈ Elo 1264, Gemini 3 Pro Image ≈ 1252. The "spanks Gemini" framing is real but the margin is single-digit Elo — not a generational gap. Where OpenAI clearly wins: prompt adherence, text rendering inside images, and instruction following on edits. Where Gemini still wins: 4K, ultra-wide aspect ratios (21:9, 4:1, 8:1), and (importantly for this codebase) returning a TEXT part alongside the image — which imagemage uses for AI-suggested filenames.

---

## Capability Gap Analysis

These are the concrete regressions if OpenAI becomes the default:

| Feature | Gemini today | OpenAI gpt-image-* | Recommendation |
|---|---|---|---|
| Default resolution | 4K | 1536px max | Default to `1024x1024`; document the regression. Keep `--resolution=4K` working only when `--provider=gemini`. |
| Aspect ratios | 1:1, 16:9, 9:16, 4:3, 3:4, 3:2, 2:3, 21:9, 5:4, 4:5, 4:1, 1:4, 8:1, 1:8 | 1:1 (1024²), 2:3 (1024×1536), 3:2 (1536×1024) only | Map closest supported size on OpenAI; print a warning when the user-requested ratio cannot be honored exactly. |
| AI-suggested filename | Returned as TEXT part by Gemini, free | Image endpoints don't return text | Drop "AI-suggested" filename in OpenAI path; fall back to existing prompt-based filename generator in `pkg/filehandler`. (Optional later: a separate cheap chat-completions call for naming.) |
| Multi-image edit | Inline base64 in JSON, up to 14 | Multipart, up to 16 | Implement multipart handling in `pkg/openai`. |
| `--frugal` model | `gemini-3.1-flash-image-preview` | Closest analogue is `gpt-image-1-mini` | Re-bind `--frugal` to `gpt-image-1-mini` under OpenAI. |
| Quality tier (`low`/`medium`/`high`) | n/a (Gemini has no equivalent flag) | First-class | Add `--quality` flag, ignored by Gemini. |
| Service-account / bearer auth | Custom transport (existing) | Bearer is the *only* auth mode | Native fit. |

The plan keeps every existing Gemini capability intact when `--provider=gemini` is selected — no Gemini features are deleted, just demoted from the default path.

---

## Architecture

```
pkg/
├── provider/         (new)   - interface + factory + ErrUnsupported sentinel
│   └── provider.go
├── gemini/                   - existing, lightly refactored to satisfy Provider
│   ├── client.go
│   ├── config.go
│   └── client_test.go
└── openai/           (new)
    ├── client.go             - /v1/images/generations + /v1/images/edits
    ├── client_test.go        - httptest-based, mirrors pattern in gemini/client_test.go
    └── multipart.go          - small helper for image edits form-data
```

Interface (intentionally narrow, mirrors what `cmd/*` already uses):

```go
package provider

import "errors"

var ErrUnsupported = errors.New("provider does not support this option")

type Result struct {
    ImageData     string // base64 PNG (or whatever the provider returns; pipeline already assumes PNG-ish)
    SuggestedName string // empty if provider has no suggestion
    Format        string // "png", "webp", etc - filehandler may need this
}

type GenerateOptions struct {
    Prompt      string
    Images      []string // base64 inputs, empty for pure generation
    AspectRatio string   // "16:9" etc; provider maps to nearest supported
    Resolution  string   // "1K"/"2K"/"4K" (Gemini) or "1024x1024" passthrough (OpenAI)
    Quality     string   // "low"/"medium"/"high" (OpenAI), ignored by Gemini
    Frugal      bool     // selects mini/flash variant
}

type Provider interface {
    Name() string                                    // "openai" | "gemini"
    Generate(ctx context.Context, opts GenerateOptions) (Result, error)
}

func New(name string, frugal bool) (Provider, error) // factory; reads env vars
```

Selection precedence:
1. `--provider` flag
2. `IMAGEMAGE_PROVIDER` env var
3. Default: `openai` (only after the user verifies the default switch is acceptable — see Task 9)

Auth env vars (additive, no breakage):
- OpenAI: `OPENAI_API_KEY` (also `IMAGEMAGE_OPENAI_API_KEY` for parity with the existing NANOBANANA_* convention).
- Gemini: existing `NANOBANANA_GEMINI_API_KEY` / `GEMINI_API_KEY` / etc., unchanged.

---

## Tasks

Each task is one commit. TDD: tests first where they pay off (the provider interface, OpenAI request shape, edit multipart). Trivial wiring tasks skip tests.

### Task 1: Add provider interface package

**Files:**
- Create: `pkg/provider/provider.go`
- Create: `pkg/provider/provider_test.go`

**Step 1: Write the failing test**

```go
// pkg/provider/provider_test.go
package provider

import "testing"

func TestNew_UnknownProvider(t *testing.T) {
    if _, err := New("nonsense", false); err == nil {
        t.Fatal("expected error for unknown provider")
    }
}

func TestErrUnsupported_IsSentinel(t *testing.T) {
    if ErrUnsupported.Error() == "" {
        t.Fatal("ErrUnsupported should have a message")
    }
}
```

**Step 2: Run** — `go test ./pkg/provider/...` expect FAIL (package missing).

**Step 3: Implement** — define `Provider`, `Result`, `GenerateOptions`, `ErrUnsupported`, and a `New` that returns `errors.New("unknown provider: ...")` for now (real registration in Tasks 2/3).

**Step 4: Run** — expect PASS.

**Step 5: Commit** — `feat(provider): add image provider interface`.

### Task 2: Wrap existing Gemini client behind Provider

**Files:**
- Modify: `pkg/gemini/client.go` — add a thin adapter type `ProviderAdapter` that translates `provider.GenerateOptions` to the existing `GenerateContentWithFullOptions` call. Do **not** refactor the client itself in this task.
- Modify: `pkg/provider/provider.go` — `New("gemini", frugal)` returns the adapter.
- Create: `pkg/gemini/provider_test.go` — verifies adapter forwards aspect ratio, resolution, images, suggested name.

**Step 1: Write a test that constructs a Gemini provider with `httptest`-injected base URL** (follow the pattern in `pkg/gemini/client_test.go:30-75`) and asserts options are forwarded.

**Step 2: Run** — FAIL (adapter missing).

**Step 3: Implement adapter:**
```go
// in pkg/gemini/client.go (or a new pkg/gemini/provider.go)
type ProviderAdapter struct{ client *Client }

func (a *ProviderAdapter) Name() string { return "gemini" }
func (a *ProviderAdapter) Generate(ctx context.Context, opts provider.GenerateOptions) (provider.Result, error) {
    res, err := a.client.GenerateContentWithFullOptions(opts.Prompt, opts.Images, opts.Resolution, opts.AspectRatio)
    return provider.Result{ImageData: res.ImageData, SuggestedName: res.SuggestedName, Format: "png"}, err
}
```

Register in `pkg/provider/provider.go` factory.

**Step 4: Run** — `go test ./...` expect PASS.

**Step 5: Commit** — `feat(provider): wrap gemini client behind Provider interface`.

### Task 3: Implement OpenAI client — generation endpoint

**Files:**
- Create: `pkg/openai/client.go`
- Create: `pkg/openai/client_test.go`

Reference: `pkg/gemini/client.go:230-413` is the closest existing pattern for retry, debug logging, and httptest-based testing.

**Step 1: Write failing tests:**
- `TestGenerate_BuildsCorrectRequest` — POST to `/v1/images/generations`, JSON body contains `model`, `prompt`, `n`, `size`, `quality`. Authorization header is `Bearer ...`.
- `TestGenerate_ParsesB64Response` — given `{"data":[{"b64_json":"..."}]}`, returns ImageData populated.
- `TestGenerate_MapsAspectRatioToSize`:
  - `"1:1"` → `1024x1024`
  - `"16:9"` and `"3:2"` → `1536x1024`
  - `"9:16"` and `"2:3"` → `1024x1536`
  - `"21:9"` → returns `1536x1024` AND a wrapped warning indicator (test for it via a `Warnings []string` field on Result, or stderr capture — pick one).
- `TestGenerate_FrugalSelectsMini` — `Frugal: true` → request body has `"model":"gpt-image-1-mini"`.
- `TestGenerate_DefaultModel` — default is `gpt-image-2` (revisit if the user wants 1.5 — see Open Decisions below).

**Step 2: Run** — FAIL.

**Step 3: Implement** — model constants, `mapAspectRatio(string) (size string, warning string)`, `Generate` method that marshals JSON, sets headers, executes via `http.Client{Timeout: 5*time.Minute}`, parses `{ data: [{ b64_json }] }`. Wire to `provider.New("openai", frugal)`.

**Step 4: Run** — PASS.

**Step 5: Commit** — `feat(openai): add image generation client`.

### Task 4: Implement OpenAI client — edits endpoint (multipart)

**Files:**
- Create: `pkg/openai/multipart.go`
- Modify: `pkg/openai/client.go` — `Generate` branches on `len(opts.Images) > 0` and calls a new `edit` path.
- Modify: `pkg/openai/client_test.go`

**Step 1: Write a failing test** — when `opts.Images` is non-empty, the request hits `/v1/images/edits`, content type starts with `multipart/form-data;`, the form contains fields `model`, `prompt`, `image[]` (one per input), and `size`. Use `multipart.NewReader` on the captured body to verify.

**Step 2: Run** — FAIL.

**Step 3: Implement** — `mime/multipart.Writer`, decode each base64 input back to bytes, write each as a `image[]` form file with PNG content type.

**Step 4: Run** — PASS.

**Step 5: Commit** — `feat(openai): support image editing/composition via multipart`.

### Task 5: Quality tier + warning surfacing

**Files:**
- Modify: `pkg/provider/provider.go` — add `Result.Warnings []string`.
- Modify: `pkg/openai/client.go` — populate Warnings when aspect ratio is mapped or resolution is downgraded.
- Modify: `pkg/gemini/client.go` adapter — leaves Warnings empty.
- Modify: `pkg/openai/client_test.go` — assert warning text for the 21:9 case.

**Step 5: Commit** — `feat(provider): expose provider warnings on Result`.

### Task 6: Wire `--provider` and `--quality` flags into commands

**Files:**
- Modify: `cmd/root.go` — add a persistent `--provider` flag (and read `IMAGEMAGE_PROVIDER`), default initially to `gemini` (we flip in Task 9).
- Modify: `cmd/generate.go` — replace `gemini.NewClient()` / `gemini.NewFrugalClient()` calls with `provider.New(providerName, generateFrugal)`. Add `--quality` flag (default empty, only meaningful for OpenAI).
- Modify: `cmd/edit.go` — same.
- Modify: `cmd/restore.go`, `cmd/icon.go`, `cmd/pattern.go`, `cmd/story.go`, `cmd/diagram.go` — use the factory; for now, force `--provider=gemini` internally for `restore` and `pattern` (those use Gemini-specific tricks like 4K + extreme aspect ratios) and document this in the help text.
- Modify: `cmd/*.go` — print `result.Warnings` to stderr after each generation.

**Step 5: Commit** — `feat(cmd): route image generation through provider abstraction`.

### Task 7: End-to-end smoke test against real APIs

**Files:**
- Create: `examples.sh` — already exists (`/home/cquinn/src/imagemage/examples.sh`); modify to add an OpenAI-flavored block that runs only when `OPENAI_API_KEY` is set.

Run by hand against both providers:
```bash
OPENAI_API_KEY=... ./imagemage generate "a fox on a moss-covered log" --provider=openai --quality=medium
OPENAI_API_KEY=... ./imagemage edit input.png "make it watercolor" --provider=openai
GEMINI_API_KEY=... ./imagemage generate "same prompt" --provider=gemini --resolution=4K --aspect-ratio=21:9
```

Confirm: PNG written, dimensions match expectations, warnings printed for ratio-mapping cases.

**Step 5: Commit** — `chore: add OpenAI smoke examples`.

### Task 8: README + help text

**Files:**
- Modify: `README.md` — add a "Providers" section near the top, document `--provider`, `--quality`, the resolution/aspect-ratio caveats, the `OPENAI_API_KEY` env var, and explicitly mark Gemini as "still supported, optional".
- Modify: `cmd/root.go` `Long` description.

**Step 5: Commit** — `docs: document OpenAI provider and provider selection`.

### Task 9: Flip the default (separate commit, easy to revert)

After hand-testing convinces us the OpenAI path is solid:

**Files:**
- Modify: `cmd/root.go` — default `--provider` becomes `openai`.
- Modify: `README.md` — adjust the headline accordingly ("Defaults to OpenAI; pass `--provider=gemini` for legacy 4K / ultra-wide / suggested-filename").

**Step 5: Commit** — `feat: default image provider to OpenAI; gemini becomes legacy opt-in`.

---

## Open Decisions (need user input before Task 3)

1. **Default OpenAI model.** `gpt-image-2` (newest, reasoning) vs `gpt-image-1.5` (4× faster, well-understood pricing, ~same quality). Recommendation: **`gpt-image-1.5`** — speed and predictable cost win for a CLI; `gpt-image-2` adds latency from reasoning.
2. **`--frugal` mapping.** Recommendation: `gpt-image-1-mini` ($0.005 floor).
3. **AI-suggested filename on OpenAI path.** Three options:
   - (a) Drop the feature, use prompt-based filename — zero extra cost, slight UX regression.
   - (b) Add a separate `gpt-4o-mini` chat call for naming — ~$0.0001/image, extra latency.
   - (c) Keep using Gemini (any tier) just for naming, even when OpenAI generates the image — weird but free if user has both keys.
   Recommendation: **(a)**, revisit only if users complain.
4. **`restore` and `pattern` commands.** They lean on Gemini-only behaviors (4K restore, ultra-wide patterns). Recommendation: hard-pin those two commands to Gemini in Task 6 and document the constraint, rather than half-supporting them on OpenAI.

---

## Risk Notes

- **Org verification gate.** OpenAI requires "API Organization Verification" before any gpt-image-* call works. If the user hasn't done it, every OpenAI request returns 403. The error handler in `pkg/openai/client.go` should detect this and print an actionable message ("Run org verification at platform.openai.com/...").
- **Cost surprise.** `gpt-image-1` high-quality at $0.167/image is meaningfully more than Imagen 4 Fast at $0.02. The default-after-flip should be `gpt-image-1.5` low or medium quality, not high — and the README should call out the price floor.
- **Test coverage.** The existing `pkg/gemini/client_test.go` covers ~5 paths; mirror that depth for `pkg/openai/`. Don't add E2E tests that require a live API key — `httptest` only.

---

## Sources

- [GPT Image — Wikipedia](https://en.wikipedia.org/wiki/GPT_Image) — model timeline (gpt-image-1 → 2)
- [AI Image Pricing 2026: Google Gemini vs. OpenAI GPT Cost Analysis — IntuitionLabs](https://intuitionlabs.ai/articles/ai-image-generation-pricing-google-openai)
- [AI Image API Pricing Comparison 2026 — LaoZhang](https://blog.laozhang.ai/en/posts/ai-image-api-pricing-comparison)
- [OpenAI Image Generation API Pricing in 2026 — AI Free API](https://www.aifreeapi.com/en/posts/openai-image-generation-api-pricing)
- [gpt-image-1 — AI/ML API docs](https://docs.aimlapi.com/api-references/image-models/openai/gpt-image-1) — endpoint and request shape examples
- [OpenAI API Reference — Create image](https://developers.openai.com/api/reference/resources/images/methods/generate)
