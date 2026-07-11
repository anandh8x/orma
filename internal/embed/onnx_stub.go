//go:build !cgo

package embed

import "fmt"

// openONNX is unavailable without cgo; callers fall back to HashEmbedder.
func openONNX(modelsDir string) (Embedder, error) {
	_ = modelsDir
	return nil, fmt.Errorf("onnx MiniLM requires a cgo-enabled build")
}
