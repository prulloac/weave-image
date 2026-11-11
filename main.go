// Command weave-image enriches image blobs with OCR, EXIF, dimensions and AI metadata.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

const (
	protocolVersion = "weave.extension/v1"
	cliVersion      = "weave.cli/v1"
	maxImageBytes   = 20 << 20
	maxInput        = 32 << 20
	maxOutput       = 10 << 20
)

var version = "dev"

type enrichmentRequest struct {
	V           string         `json:"v"`
	Type        string         `json:"type"`
	DocID       string         `json:"doc_id"`
	DocPath     string         `json:"doc_path"`
	BundleRoot  string         `json:"bundle_root"`
	Title       string         `json:"title"`
	MediaType   string         `json:"media_type"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

type enrichmentResponse struct {
	V                       string         `json:"v"`
	InferredTitle           string         `json:"inferred_title,omitempty"`
	InferredDescription     string         `json:"inferred_description,omitempty"`
	InferredText            string         `json:"inferred_text,omitempty"`
	InferredTextSource      string         `json:"inferred_text_source,omitempty"`
	InferredContentMetadata map[string]any `json:"inferred_content_metadata,omitempty"`
	InferredKeywords        []string       `json:"inferred_keywords,omitempty"`
	Warnings                []string       `json:"warnings,omitempty"`
	CustomMetadata          map[string]any `json:"custom_metadata,omitempty"`
}

type manifest struct {
	V            string        `json:"v"`
	Name         string        `json:"name"`
	AssetTypes   []string      `json:"asset_types"`
	Requires     []requirement `json:"requires"`
	Capabilities []string      `json:"capabilities"`
}

type requirement struct {
	Name       string `json:"name"`
	MinVersion string `json:"min_version"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return enrichCommand(stdin, stdout, stderr)
	}
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintf(stdout, "weave-image %s (%s)\n", version, protocolVersion)
		return 0
	case "--describe", "describe":
		return describe(args[1:], stdout, stderr)
	case "enrich", "--enrich":
		return enrichCommand(stdin, stdout, stderr)
	default:
		return fail(stderr, "unknown command: "+args[0])
	}
}

func describe(args []string, stdout, stderr io.Writer) int {
	value := manifest{
		V:          protocolVersion,
		Name:       "image",
		AssetTypes: []string{"image/jpeg", "image/png", "image/webp", "image/gif"},
		Requires: []requirement{
			{Name: "weave", MinVersion: "0.8.0"},
			{Name: "weave-ai", MinVersion: "0.8.0"},
		},
		Capabilities: []string{"vision", "ocr", "exif"},
	}
	for _, arg := range args {
		if arg == "--json" {
			if err := json.NewEncoder(stdout).Encode(value); err != nil {
				return fail(stderr, "cannot encode manifest")
			}
			return 0
		}
	}
	fmt.Fprintf(stdout, "name: image\nasset_types: %s\nrequires: weave >= 0.8.0, weave-ai >= 0.8.0\n",
		strings.Join(value.AssetTypes, ", "))
	fmt.Fprintln(stdout, "capabilities: vision, ocr, exif")
	fmt.Fprintln(stdout, "note: EXIF is sent to the configured AI provider")
	return 0
}

func enrichCommand(stdin io.Reader, stdout, stderr io.Writer) int {
	data, err := io.ReadAll(io.LimitReader(stdin, maxInput+1))
	if err != nil || len(data) > maxInput {
		return fail(stderr, "invalid input")
	}
	var request enrichmentRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return fail(stderr, "invalid enrichment request")
	}
	result, err := enrich(context.Background(), request)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		return fail(stderr, "cannot encode response")
	}
	return 0
}

func enrich(ctx context.Context, request enrichmentRequest) (enrichmentResponse, error) {
	path, err := imagePath(request)
	if err != nil {
		return enrichmentResponse{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return enrichmentResponse{}, fmt.Errorf("cannot read image")
	}
	if info.Size() > maxImageBytes {
		return enrichmentResponse{}, fmt.Errorf("image too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return enrichmentResponse{}, fmt.Errorf("cannot read image")
	}
	mediaType := request.MediaType
	if mediaType == "" {
		mediaType = detectMediaType(path, data)
	}
	if !strings.HasPrefix(mediaType, "image/") {
		return enrichmentResponse{V: cliVersion}, nil
	}
	dimensions, exif := localMetadata(path, data, mediaType)
	ocrText, warning := runOCR(ctx, path)
	warnings := make([]string, 0)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	vision, visionWarning := callVision(ctx, request, path, mediaType, data, ocrText, exif)
	if visionWarning != "" {
		warnings = append(warnings, visionWarning)
	}

	text, textSource := "", ""
	if strings.TrimSpace(ocrText) != "" {
		text, textSource = strings.TrimSpace(ocrText), "ocr"
	} else if strings.TrimSpace(vision.Text) != "" {
		text, textSource = strings.TrimSpace(vision.Text), "vision"
	}
	attributes := map[string]any{}
	if dimensions.Width > 0 {
		attributes["width"] = dimensions.Width
	}
	if dimensions.Height > 0 {
		attributes["height"] = dimensions.Height
	}
	if len(exif) > 0 {
		attributes["exif"] = exif
	}
	metadata := map[string]any{"v": 1}
	if len(attributes) > 0 {
		metadata["attributes"] = attributes
	}
	if text != "" {
		metadata["text"] = text
		metadata["text_source"] = textSource
	}
	custom := map[string]any{"source": textSource, "vision": len(vision.Keywords) > 0 || vision.Title != ""}
	return enrichmentResponse{
		V:                       cliVersion,
		InferredTitle:           vision.Title,
		InferredDescription:     vision.Description,
		InferredText:            text,
		InferredTextSource:      textSource,
		InferredContentMetadata: metadata,
		InferredKeywords:        vision.Keywords,
		Warnings:                warnings,
		CustomMetadata:          custom,
	}, nil
}

func imagePath(request enrichmentRequest) (string, error) {
	if request.BundleRoot == "" || request.DocPath == "" {
		return "", fmt.Errorf("bundle_root and doc_path are required")
	}
	root, err := filepath.Abs(request.BundleRoot)
	if err != nil {
		return "", fmt.Errorf("invalid bundle root")
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(request.DocPath)))
	if err != nil {
		return "", fmt.Errorf("invalid document path")
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("document path escapes the bundle")
	}
	return path, nil
}

func localMetadata(path string, data []byte, mediaType string) (imageDimensions, map[string]any) {
	dimensions := imageDimensions{}
	if file, err := os.Open(path); err == nil {
		if cfg, _, err := image.DecodeConfig(file); err == nil {
			dimensions.Width = cfg.Width
			dimensions.Height = cfg.Height
		}
		file.Close()
	}
	var exif map[string]any
	if mediaType == "image/jpeg" {
		exif = parseJPEGExif(data)
	}
	return dimensions, exif
}

func detectMediaType(path string, data []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	if len(data) >= 4 && string(data[:4]) == "\x89PNG" {
		return "image/png"
	}
	return "application/octet-stream"
}

func runOCR(ctx context.Context, path string) (string, string) {
	binary, err := lookPath("tesseract")
	if err != nil {
		return "", ""
	}
	language := os.Getenv("WEAVE_IMAGE_OCR_LANG")
	if language == "" {
		language = "eng"
	}
	runContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stdout, stderr, err := runProcess(runContext, binary, []string{path, "stdout", "-l", language}, nil)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", "local OCR failed"
		}
		return "", ""
	}
	return strings.TrimSpace(string(stdout)), ""
}

func callVision(
	ctx context.Context,
	request enrichmentRequest,
	path, mediaType string,
	data []byte,
	ocrText string,
	exif map[string]any,
) (visionResponse, string) {
	binary, warning := resolveWeaveAI(request.BundleRoot)
	if binary == "" {
		return visionResponse{}, warning
	}
	if warning = checkWeaveAIVersion(ctx, binary); warning != "" {
		return visionResponse{}, warning
	}
	payload := map[string]any{
		"v":          "weave.cli/v1",
		"type":       "vision_request",
		"media_type": mediaType,
		"image":      base64.StdEncoding.EncodeToString(data),
		"text":       ocrText,
		"prompt":     "Return JSON with title, description, keywords and optional text.",
	}
	if len(exif) > 0 {
		payload["exif"] = exif
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return visionResponse{}, "cannot encode vision request"
	}
	timeout := 60 * time.Second
	if value := os.Getenv("WEAVE_IMAGE_AI_TIMEOUT"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, _, err := runProcess(runContext, binary, nil, append(encoded, '\n'))
	if err != nil {
		return visionResponse{}, "weave-ai vision request failed"
	}
	var response visionResponse
	if err := json.Unmarshal(stdout, &response); err != nil {
		return visionResponse{}, "weave-ai returned invalid JSON"
	}
	return response, ""
}

type visionResponse struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Text        string   `json:"text"`
	Keywords    []string `json:"keywords"`
}

func fail(stderr io.Writer, message string) int {
	fmt.Fprintf(stderr, "weave-image: %s\n", message)
	return 1
}
