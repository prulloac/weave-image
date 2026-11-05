package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const minimumAI = "1.1.0"

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func resolveWeaveAI(bundleRoot string) (string, string) {
	if bundleRoot != "" {
		path := filepath.Join(bundleRoot, ".weave", "bin", "weave-ai")
		if executable(path) {
			return path, ""
		}
	}
	path, err := exec.LookPath("weave-ai")
	if err != nil {
		return "", "weave-ai not found; install weave-ai >= " + minimumAI
	}
	return path, ""
}

func checkWeaveAIVersion(ctx context.Context, binary string) string {
	runContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stdout, _, err := runProcess(runContext, binary, []string{"--version"}, nil)
	if err != nil {
		return "weave-ai version check failed; require >= " + minimumAI
	}
	text := strings.TrimSpace(string(stdout))
	if strings.Contains(text, "dev") {
		return ""
	}
	match := versionPattern.FindStringSubmatch(text)
	if len(match) != 4 {
		return "weave-ai version is unrecognized; require >= " + minimumAI
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	required := []int{1, 1, 0}
	actual := []int{major, minor, patch}
	for index := range required {
		if actual[index] > required[index] {
			return ""
		}
		if actual[index] < required[index] {
			return fmt.Sprintf("weave-ai %d.%d.%d is too old; require >= %s", major, minor, patch, minimumAI)
		}
	}
	return ""
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
