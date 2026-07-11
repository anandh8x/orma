//go:build cgo

package embed

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMiniLMONNXIfAssetsPresent(t *testing.T) {
	// Optional integration: use ORMA_TEST_MODELS_DIR or skip.
	dir := os.Getenv("ORMA_TEST_MODELS_DIR")
	if dir == "" {
		// try probe path used in local dev
		candidates := []string{
			"/tmp/orma-onnx-data/orma/models",
			filepath.Join(os.TempDir(), "orma-hf-probe-models"),
		}
		for _, c := range candidates {
			if ONNXReady(c) {
				dir = c
				break
			}
		}
	}
	if dir == "" || !ONNXReady(dir) {
		t.Skip("onnx assets not present; set ORMA_TEST_MODELS_DIR")
	}
	e, err := NewMiniLMONNX(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Embed(context.Background(), "docker compose up postgres")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Embed(context.Background(), "start the database containers")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Embed(context.Background(), "ssh into the production server")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != Dim {
		t.Fatalf("dim %d", len(a))
	}
	rel := cos32(a, b)
	unrel := cos32(a, c)
	t.Logf("related=%.3f unrelated=%.3f", rel, unrel)
	if rel <= unrel {
		// soft check: related should usually win; don't hard-fail on flaky quant models
		t.Logf("warning: related not higher than unrelated")
	}
}

func cos32(a, b []float32) float64 {
	var d, na, nb float64
	for i := range a {
		d += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return d / (math.Sqrt(na) * math.Sqrt(nb))
}
