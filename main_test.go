package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string) {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 2, 3))
	value.Set(0, 0, color.RGBA{R: 255, A: 255})
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, value); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func invoke(t *testing.T, args []string, input []byte) (int, []byte, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, bytes.NewReader(input), &stdout, &stderr)
	return code, stdout.Bytes(), stderr.String()
}

func enrichmentInput(t *testing.T, root string) []byte {
	t.Helper()
	value := map[string]any{
		"v": "weave.cli/v1", "type": "enrichment_request",
		"doc_id": "photo.png", "doc_path": "photo.png", "bundle_root": root,
		"title": "", "media_type": "image/png", "frontmatter": map[string]any{}, "body": "",
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDescribeJSON(t *testing.T) {
	code, output, stderr := invoke(t, []string{"--describe", "--json"}, nil)
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var value manifest
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	if value.V != protocolVersion || value.Name != "image" || len(value.AssetTypes) != 4 ||
		len(value.Capabilities) != 3 {
		t.Fatalf("manifest = %+v", value)
	}
	if len(value.Requires) != 2 || value.Requires[1].MinVersion != "1.1.0" {
		t.Fatalf("requirements = %+v", value.Requires)
	}
}

func TestVisionAndOCRPrecedence(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "photo.png"))
	bin := filepath.Join(t.TempDir(), "bin")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\necho 'Opening hours'\n")
	writeExecutable(t, filepath.Join(root, ".weave", "bin", "weave-ai"), `#!/bin/sh
if [ "$1" = "--version" ]; then echo "weave-ai v1.1.0 (weave.cli/v1)"; exit 0; fi
cat >/dev/null
echo '{"v":"weave.cli/v1","title":"Shop","description":"A storefront","text":"AI text","keywords":["shop","sign"]}'
`)
	t.Setenv("PATH", bin+":/usr/bin:/bin")

	code, output, stderr := invoke(t, []string{"enrich"}, enrichmentInput(t, root))
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var response enrichmentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	if response.InferredTitle != "Shop" || response.InferredDescription != "A storefront" {
		t.Fatalf("AI metadata missing: %+v", response)
	}
	if response.InferredText != "Opening hours" || response.InferredTextSource != "ocr" {
		t.Fatalf("OCR precedence broken: %+v", response)
	}
	if len(response.InferredKeywords) != 2 {
		t.Fatalf("keywords = %v", response.InferredKeywords)
	}
	attributes, _ := response.InferredContentMetadata["attributes"].(map[string]any)
	if attributes["width"] != float64(2) || attributes["height"] != float64(3) {
		t.Fatalf("dimensions = %+v", attributes)
	}
	if len(response.Warnings) != 0 {
		t.Fatalf("warnings = %v", response.Warnings)
	}
}

func TestMissingWeaveAIWarnsAndKeepsLocalText(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "photo.png"))
	bin := filepath.Join(t.TempDir(), "bin")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\necho 'Local text'\n")
	t.Setenv("PATH", bin)

	code, output, stderr := invoke(t, []string{"enrich"}, enrichmentInput(t, root))
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var response enrichmentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	if response.InferredText != "Local text" || response.InferredTextSource != "ocr" {
		t.Fatalf("local text missing: %+v", response)
	}
	if len(response.Warnings) == 0 || !strings.Contains(strings.Join(response.Warnings, " "), "weave-ai") {
		t.Fatalf("warnings = %v", response.Warnings)
	}
}

func TestOldWeaveAIWarns(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "photo.png"))
	writeExecutable(t, filepath.Join(root, ".weave", "bin", "weave-ai"), `#!/bin/sh
if [ "$1" = "--version" ]; then echo "weave-ai v1.0.0 (weave.cli/v1)"; exit 0; fi
cat >/dev/null
echo '{"v":"weave.cli/v1"}'
`)
	t.Setenv("PATH", "/usr/bin:/bin")
	code, output, _ := invoke(t, []string{"enrich"}, enrichmentInput(t, root))
	if code != 0 {
		t.Fatal("expected success with warning")
	}
	var response enrichmentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Warnings) == 0 || !strings.Contains(strings.Join(response.Warnings, " "), "too old") {
		t.Fatalf("warnings = %v", response.Warnings)
	}
}

func TestParseJPEGExif(t *testing.T) {
	tiff := make([]byte, 0, 64)
	tiff = append(tiff, 'I', 'I')
	tiff = binary.LittleEndian.AppendUint16(tiff, 42)
	tiff = binary.LittleEndian.AppendUint32(tiff, 8)
	tiff = binary.LittleEndian.AppendUint16(tiff, 2)
	makeEntry := make([]byte, 12)
	binary.LittleEndian.PutUint16(makeEntry[0:], 0x010F)
	binary.LittleEndian.PutUint16(makeEntry[2:], 2)
	binary.LittleEndian.PutUint32(makeEntry[4:], 6)
	binary.LittleEndian.PutUint32(makeEntry[8:], 38)
	modelEntry := make([]byte, 12)
	binary.LittleEndian.PutUint16(modelEntry[0:], 0x0110)
	binary.LittleEndian.PutUint16(modelEntry[2:], 2)
	binary.LittleEndian.PutUint32(modelEntry[4:], 5)
	binary.LittleEndian.PutUint32(modelEntry[8:], 44)
	tiff = append(tiff, makeEntry...)
	tiff = append(tiff, modelEntry...)
	tiff = binary.LittleEndian.AppendUint32(tiff, 0)
	tiff = append(tiff, []byte("Apple\x00X100\x00")...)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	length := len(payload) + 2
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	jpeg = binary.BigEndian.AppendUint16(jpeg, uint16(length))
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xD9)

	exif := parseJPEGExif(jpeg)
	if exif["Make"] != "Apple" || exif["Model"] != "X100" {
		t.Fatalf("exif = %+v", exif)
	}
}

func TestDirectInvocation(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "photo.png"))
	t.Setenv("PATH", "/usr/bin:/bin")
	code, output, stderr := invoke(t, nil, enrichmentInput(t, root))
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	var response enrichmentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	if response.V != cliVersion {
		t.Fatalf("version = %q", response.V)
	}
}

func TestVersionOutput(t *testing.T) {
	original := version
	version = "v9.9.9"
	t.Cleanup(func() { version = original })
	code, output, stderr := invoke(t, []string{"--version"}, nil)
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if got, want := string(output), "weave-image v9.9.9 (weave.extension/v1)\n"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}
