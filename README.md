# weave-image

First-party image enrichment extension for [Weave](https://github.com/prulloac/weave).

`weave-image` turns image blobs into searchable documents. It extracts OCR text, EXIF and dimensions
locally, then asks [`weave-ai`](https://github.com/prulloac/weave-ai) for a vision title, description,
keywords and optional transcription. Core stays media-agnostic.

## Install

User-level installation, no admin or sudo required. Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/prulloac/weave-image/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/prulloac/weave-image/main/scripts/install.ps1 | iex
```

The installer:

- installs `weave-image` to a user-level bin directory;
- checks `weave >= 1.1.0` at runtime;
- installs or upgrades `weave-ai` unless `--no-ai` is passed;
- verifies `weave-ai >= 1.1.0` for vision.

`--bin-dir <dir>` and `--no-modify-path` are supported. Environment equivalents:
`WEAVE_IMAGE_VERSION`, `WEAVE_IMAGE_BIN_DIR`, `WEAVE_IMAGE_NO_MODIFY_PATH=1`, `WEAVE_IMAGE_NO_AI=1`.

## Use

```sh
weave ingest ~/photos --assets --with-enricher image --db ~/.weave/index.db
weave search ~/photos "opening hours" --db ~/.weave/index.db
weave show ~/photos photo.png --db ~/.weave/index.db --out photo.png
```

- Supported media types: `image/jpeg`, `image/png`, `image/webp`, `image/gif`.
- OCR uses Tesseract when it is installed; otherwise `weave-ai` transcribes.
- EXIF and dimensions are always kept; EXIF is sent to the configured AI provider as context.
- AI fills only empty title/description and appends keywords.
- Missing or old `weave-ai`, provider failures and malformed responses keep the local metadata and
  add warnings; the image document is still stored.
- `--ocr-lang` is configured with `WEAVE_IMAGE_OCR_LANG` (default `eng`).

## Privacy

Images and EXIF are sent directly to the configured `weave-ai` provider. Core text redaction does not
apply to vision requests, so enable the image enricher deliberately. `weave-image --describe` states
that EXIF is sent to the provider. Errors never include image bytes or EXIF values.

## Protocol

`weave-image` is a `weave.cli/v1` extension invoked by core as an enricher. It reads
`bundle_root`/`doc_path` directly; image bytes never travel through core IPC.

### Capability manifest

```json
{
  "v": "weave.extension/v1",
  "name": "image",
  "asset_types": ["image/jpeg", "image/png", "image/webp", "image/gif"],
  "requires": [
    {"name": "weave", "min_version": "1.1.0"},
    {"name": "weave-ai", "min_version": "1.1.0"}
  ],
  "capabilities": ["vision", "ocr", "exif"]
}
```

### Enrichment response

The extension returns:

- `inferred_title`, `inferred_description` from vision;
- `inferred_text` and `inferred_text_source` (`ocr` or `vision`);
- `inferred_content_metadata` with `text`, `text_source`, `width`, `height` and `exif`;
- `inferred_keywords`;
- `warnings` for missing/old AI or provider failures;
- `custom_metadata` with source and vision flags.

Local OCR text always wins over AI transcription.

## Development

```sh
./scripts/verify.sh
go build -o weave-image .
```

Tests build fake `tesseract` and `weave-ai` binaries; no real OCR or provider is called.
Behavior examples live in [`docs/bdd/image.md`](docs/bdd/image.md). The authoritative contract is
[`weave-image.md`](https://github.com/prulloac/weave-docs/blob/main/specs/weave-image.md).

## Compatibility

- `weave >= 1.1.0`
- `weave-ai >= 1.1.0`

## License

MIT. See [`LICENSE`](LICENSE).
