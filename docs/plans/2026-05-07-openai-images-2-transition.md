# OpenAI Images 2 Transition Plan

Date: 2026-05-07

## Executive Summary

Imagemage is currently a Gemini-specific Go CLI. The core image API surface is concentrated in `pkg/gemini/client.go`, while all commands import that package directly. OpenAI's current image docs identify `gpt-image-2` as the latest GPT Image model and support both text-to-image generation and image editing through the Images API. The right transition is not a blind provider swap. First add a provider abstraction and eval harness, then make OpenAI the default provider once local quality, cost, and command parity pass.

Recommendation: migrate to a provider-neutral core with OpenAI Images API as the default provider, keep Gemini as a fallback provider for one release, then decide whether to remove Gemini-specific naming and flags.

## Research Notes

### OpenAI Images

- Current OpenAI docs state the API can generate and edit images using GPT Image models, including latest `gpt-image-2`.
- OpenAI exposes two useful paths:
  - Images API for one-shot generate/edit requests.
  - Responses API image generation tool for multi-turn editing and conversational workflows.
- For this CLI, start with the Images API because the existing commands are one-shot operations.
- `gpt-image-2` supports generation via `POST /v1/images/generations` and editing via `POST /v1/images/edits`.
- OpenAI output customization maps well to this CLI: `size`, `quality`, `format`, and compression are supported. `gpt-image-2` accepts many valid `size` resolutions up to a maximum edge of 3840px, including common 4K landscape and portrait sizes.
- `gpt-image-2` does not currently support transparent backgrounds, so icon workflows must either use `gpt-image-1.5` or post-process backgrounds when transparency is required.
- For edits/reference-image workflows, OpenAI says `gpt-image-2` processes image inputs at high fidelity automatically, so `input_fidelity` should be omitted.
- OpenAI may require API organization verification before GPT Image model access.

Sources:

- OpenAI image generation guide: https://developers.openai.com/api/docs/guides/image-generation
- OpenAI Images API reference: https://platform.openai.com/docs/api-reference/images/overview

### Gemini Baseline

- Current code uses Gemini REST `generateContent` with `generationConfig.imageConfig.aspectRatio` and `imageSize`.
- Default model is `gemini-3-pro-image-preview`; frugal model is `gemini-3.1-flash-image-preview`.
- Google docs position Gemini 3 Pro Image Preview for professional asset production, complex instructions, grounding, thinking, and up to 4K output.
- Gemini supports up to 14 reference images, with separate object and character consistency guidance.
- Google says generated images include a SynthID watermark.

Sources:

- Gemini image generation guide: https://ai.google.dev/gemini-api/docs/image-generation
- Gemini Nano Banana guide redirect: https://ai.google.dev/gemini-api/docs/nanobanana

## Current Project Impact

Provider coupling is high but localized:

- `pkg/gemini/client.go` owns auth, model selection, request/response schemas, aspect-ratio validation, retries, and error handling.
- Commands import `imagemage/pkg/gemini` directly:
  - `generate`
  - `edit`
  - `restore`
  - `icon`
  - `pattern`
  - `story`
  - `diagram`
- `--frugal` is Gemini-specific and should become a provider/model quality concept.
- `--resolution` currently accepts Gemini labels (`512px`, `1K`, `2K`, `4K`) but OpenAI Images wants concrete `WIDTHxHEIGHT` sizes or `auto`.
- `--aspect-ratio` currently maps naturally to Gemini, but OpenAI requires conversion to exact dimensions.
- Filename generation is coupled to Gemini returning text and image in the same response. OpenAI image generation responses do not supply a separate short filename suggestion, so filenames should be generated locally or via a separate text model only when explicitly enabled.

## Target Architecture

Add a provider-neutral image package:

```go
package imagegen

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderGemini Provider = "gemini"
)

type Request struct {
	Prompt       string
	ImagesBase64 []ImageInput
	AspectRatio  string
	Resolution   string
	Quality      string
	OutputFormat string
	Transparent  bool
}

type ImageInput struct {
	MimeType string
	Base64   string
}

type Result struct {
	ImageBase64   string
	SuggestedName string
	Provider      Provider
	Model         string
}

type Client interface {
	Generate(ctx context.Context, req Request) (Result, error)
	Edit(ctx context.Context, req Request) (Result, error)
}
```

Implement:

- `pkg/imagegen`: shared interfaces, validation, aspect-ratio-to-size mapping, provider selection.
- `pkg/openai`: OpenAI Images API implementation.
- Keep `pkg/gemini` initially, adapted to the shared interface.

## API Mapping

### Generate

Existing:

- Gemini endpoint: `models/{model}:generateContent`
- Prompt plus optional filename suffix.
- `generationConfig.responseModalities = ["TEXT", "IMAGE"]`
- `imageConfig.aspectRatio`, `imageConfig.imageSize`

OpenAI:

- Endpoint: `POST https://api.openai.com/v1/images/generations`
- Default model: `gpt-image-2`
- Body:
  - `model`
  - `prompt`
  - `size`
  - `quality`
  - `output_format`
  - `output_compression` when `jpeg`/`webp`
  - `n`

### Edit / Restore / Icon-from-Input

Existing:

- Same Gemini `generateContent` endpoint with inline image parts.
- Accepts base plus additional images.

OpenAI:

- Endpoint: `POST https://api.openai.com/v1/images/edits`
- Use multipart uploads first, because local files are already available and it avoids a separate upload step.
- Send repeated `image=@file` parts or JSON `images` if using file IDs later.
- Use `model=gpt-image-2` for general edits.
- Omit `input_fidelity` for `gpt-image-2`.
- Add mask support later as a new feature, not required for parity.

## CLI Changes

Phase 1 should preserve current user workflows while adding OpenAI:

- Add `--provider openai|gemini`, default from `IMAGEMAGE_PROVIDER`, then default to `openai`.
- Add `--model`, default from provider:
  - OpenAI quality default: `gpt-image-2`
  - OpenAI transparent/icon default: `gpt-image-1.5` until `gpt-image-2` supports transparency
  - Gemini default: current `gemini-3-pro-image-preview`
- Replace `--frugal` internally with `--quality low|medium|high|auto`; keep `--frugal` as deprecated alias for `--quality low`.
- Keep `--aspect-ratio`, but translate it for OpenAI using a deterministic table:
  - `1:1` + `4K` -> `2048x2048` by default, with opt-in `3840x3840` only if accepted by current constraints and cost is acceptable.
  - `16:9` + `4K` -> `3840x2160`
  - `9:16` + `4K` -> `2160x3840`
  - `16:9` + `2K` -> `2048x1152`
  - `1:1` + `1K` -> `1024x1024`
- Add `--format png|jpeg|webp`, default `png`.
- Add `--transparent` for icons and assets, selecting a compatible model/provider.

## Implementation Phases

### Phase 0: Evaluation Harness

Goal: prove whether OpenAI actually outperforms Gemini for this CLI's real jobs.

Tasks:

- Add `docs/evals/image-prompts.json` with 20 prompts covering:
  - slide images
  - diagrams with legible text
  - icons
  - photo restoration
  - reference-image composition
  - pattern generation
  - story consistency
- Add `imagemage eval` or a `make eval-images` script that runs the same prompts against providers and writes outputs into timestamped folders.
- Track: latency, API status, provider/model, requested size, output dimensions, file size, and prompt metadata.
- Manual scoring rubric: instruction following, text rendering, composition fidelity, artifact rate, editing faithfulness, visual appeal, and cost.

Exit criteria: OpenAI beats or ties Gemini on the workflows the project actually documents.

### Phase 1: Provider Abstraction

Tasks:

- Create `pkg/imagegen`.
- Move shared types out of `pkg/gemini` without changing behavior.
- Update commands to depend on `imagegen.Client`.
- Keep Gemini as the only implementation until tests pass.
- Add unit tests for provider selection and request mapping.

Exit criteria: no behavior change; `go test ./...` passes.

### Phase 2: OpenAI Provider

Tasks:

- Add `pkg/openai`.
- Auth via `OPENAI_API_KEY`.
- Implement generation with JSON `POST /v1/images/generations`.
- Implement editing with multipart `POST /v1/images/edits`.
- Add OpenAI error parsing for 400, 401, 403, 429, and 5xx.
- Add retry policy for transient 429/5xx, respecting any retry headers.
- Add tests using `httptest` to verify request bodies, multipart file handling, auth headers, and response extraction.

Exit criteria: `generate`, `edit`, `restore`, `pattern`, `story`, and `diagram` work with `--provider openai`.

### Phase 3: Default Switch

Tasks:

- Change default provider to OpenAI.
- Update README, root command copy, examples, and troubleshooting.
- Keep Gemini docs under a fallback/provider section.
- Deprecate Gemini-specific env var primacy in user-facing docs, but do not remove support.
- Add migration notes:
  - set `OPENAI_API_KEY`
  - use `IMAGEMAGE_PROVIDER=gemini` to retain old behavior
  - replace `--frugal` with `--quality low`

Exit criteria: default install path requires OpenAI key, while Gemini users have a documented compatibility path.

### Phase 4: Cleanup

Tasks:

- Rename `pkg/gemini` internals only after one compatibility release.
- Decide whether to retain Gemini as a supported secondary provider.
- Remove stale "Nano Banana" branding from README unless Gemini remains first-class.
- If OpenAI remains superior in evals, consider a `gemini` build tag or plugin-style provider later.

## Risks and Decisions

- Transparency: `gpt-image-2` currently does not support transparent backgrounds. Icon generation must select `gpt-image-1.5`, fall back to Gemini, or post-process backgrounds.
- Filename suggestions: Gemini's interleaved text output currently gives filename suggestions. OpenAI image responses should use local deterministic filenames unless a separate text model call is added.
- Cost: `gpt-image-2` cost varies by input tokens, image input tokens for edits, quality, and size. Do not default everything to 4K high without eval evidence.
- Multi-image parity: Gemini documents up to 14 reference images. OpenAI supports reference-image edit workflows, but exact limits and request forms must be covered by tests and docs before promising parity.
- Safety and access: OpenAI organization verification may block some users. Keep provider selection and clearer auth errors.
- Current Gemini frugal model string may be ahead of older public examples; validate against live Google model list before changing it.

## Proposed Defaults

- Provider: `openai`
- Model: `gpt-image-2`
- Quality: `medium`
- Size: `auto` unless `--aspect-ratio` or `--resolution` is provided
- Format: `png`
- `generate --slide`: `16:9`, `3840x2160`, `quality=medium`
- `diagram`: `quality=high` by default, because text rendering matters
- `icon`: `gpt-image-1.5` with `--transparent`, otherwise `gpt-image-2`
- `story`: `quality=medium`, avoid 4K unless requested

## First PR Scope

Keep the first PR small:

1. Add `pkg/imagegen` interfaces and provider selection.
2. Adapt Gemini to that interface.
3. Add OpenAI generate-only support.
4. Wire `generate --provider openai`.
5. Add tests for provider selection, OpenAI generation request mapping, and response parsing.
6. Update README with experimental OpenAI provider usage.

Then ship edit/restore/icon/pattern/story/diagram support in follow-up PRs.
