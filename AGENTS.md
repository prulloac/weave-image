# AGENTS.md

`weave-image` is the first-party image enrichment extension for Weave. It enriches image blob
documents with OCR, EXIF, dimensions, title, description and keywords. Core stays media-agnostic.

## Commands

- Build: `go build -o weave-image .`
- All tests: `go test ./...`
- Race: `go test -race -count=1 -shuffle=on ./...`
- Vet / format: `go vet ./...`; `gofmt -l .` must print nothing
- Local describe: `go run . --describe --json`
- Version: `go run . --version`
- Full gate: `./scripts/verify.sh`
- User-level install: `./scripts/install.sh --help` (or `./scripts/install.ps1` on Windows)

## Layout

```text
main.go            CLI, enrichment request/response, orchestration
exif.go            JPEG/TIFF EXIF parser
process.go         bounded subprocess helper
version.go         weave-ai discovery and semver check
main_test.go       protocol, OCR, vision, EXIF and fallback tests
docs/bdd/          behavior scenarios for this binary
scripts/           verification, installers and release helpers
```

## Invariants

1. **Spec first:** behavior changes need a spec update in `weave-docs`.
2. **Core stays generic:** never add image branches to `weave`.
3. **Best-effort enrichment:** missing AI, OCR or EXIF must keep local metadata and return a valid
   response with warnings, not fail the document.
4. **OCR precedence:** local OCR text wins over AI transcription.
5. **Declared metadata wins:** AI fills only empty title/description and appends keywords.
6. **No secrets:** errors and warnings never include image bytes, EXIF values or API keys.
7. **Bounded input:** images are capped at 20 MiB and subprocess output at 10 MiB.
8. **No database access:** the extension reads the source image and writes only its response.
9. **User-level install:** installers never require sudo or administrator rights.

## Testing quirks

- Tests build fake `tesseract` and `weave-ai` binaries in temporary directories and prepend them to
  `PATH`; no real OCR or provider is called.
- JPEG EXIF is covered by a handcrafted APP1 fixture.
- `TestVersionOutput` pins `weave-image <tag> (weave.extension/v1)`.

## Release

- Tags use `vX.Y.Z`; release archives are built for six static targets.
- Binary name is `weave-image`; Weave discovers it by the `weave-` prefix.
- `requires` names `weave >= 0.7.0` and `weave-ai >= 0.7.0`.

## Documentation split

- This repository owns install/usage docs and extension-side BDD scenarios.
- Core blob and image behavior lives in
  [`weave-docs/specs/weave-image.md`](https://github.com/prulloac/weave-docs/blob/main/specs/weave-image.md).
