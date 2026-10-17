package dto_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDTOOnlyDependsOnGoStandardLibrary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("dependency check: %v %s", err, output)
	}
	for _, dep := range strings.Fields(string(output)) {
		if dep == "media-tool/internal/modules/embyapi/dto" {
			continue
		}
		if strings.Contains(strings.Split(dep, "/")[0], ".") || strings.HasPrefix(dep, "media-tool/") || strings.Contains(dep, "mediaprobe") || strings.Contains(dep, "ffmpeg") || strings.Contains(dep, "ffprobe") || dep == "os/exec" {
			t.Fatal("DTO imports an adapter/database/processor", dep)
		}
	}
}
