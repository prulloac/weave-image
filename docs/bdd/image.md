# BDD: Weave Image Extension

Behavior cases for the `weave-image` subprocess.

**Spec reference:** [weave-image.md](https://github.com/prulloac/weave-docs/blob/main/specs/weave-image.md)
**Go tests:** `main_test.go`

---

## Feature: Capability and Dependency

### Scenario: Capability manifest

- **When** I run `weave-image --describe --json`
- **Then** it returns `weave.extension/v1`
- **And** `asset_types` contains JPEG, PNG, WebP and GIF
- **And** `requires` names `weave >= 1.0.0` and `weave-ai >= 1.0.0`

### Scenario: Missing weave-ai still enriches locally

- **Given** no `weave-ai` on disk
- **When** an image is enriched
- **Then** the response contains local OCR, EXIF and dimensions
- **And** warnings name the missing `weave-ai`
- **And** the response is still valid

### Scenario: Old weave-ai is rejected

- **Given** `weave-ai` reports `v0.9.0`
- **When** an image is enriched
- **Then** vision is skipped
- **And** warnings name the required version

---

## Feature: Local Metadata

### Scenario: Local OCR wins

- **Given** Tesseract returns text and `weave-ai` returns different text
- **When** the image is enriched
- **Then** `inferred_text` is the OCR text
- **And** `text_source` is `ocr`

### Scenario: Vision transcription fills an OCR gap

- **Given** no Tesseract and an AI response containing text
- **When** the image is enriched
- **Then** `inferred_text` is the vision text
- **And** `text_source` is `vision`

### Scenario: EXIF and dimensions

- **Given** a JPEG with EXIF
- **When** the image is enriched
- **Then** `attributes.exif` contains the normalized EXIF
- **And** width and height are present
- **And** the same EXIF is sent to `weave-ai`

---

## Feature: AI Metadata

### Scenario: AI fills empty fields

- **Given** an image with empty title and description
- **When** `weave-ai` returns a title and description
- **Then** both are returned as inferred values

### Scenario: Local metadata survives AI failure

- **Given** `weave-ai` times out or returns malformed JSON
- **When** the image is enriched
- **Then** local OCR, EXIF and dimensions remain
- **And** warnings describe the failure

### Scenario: Version output

- **When** I run `weave-image --version`
- **Then** it prints the binary version and `weave.extension/v1`
