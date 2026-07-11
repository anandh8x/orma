package embed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Dim is MiniLM embedding size.
const Dim = 384

// ModelName is the preferred embeddings.model id when ONNX is active.
// Kept for callers; prefer ActiveModelName(modelsDir).
const ModelName = ONNXModelName

// Embedder turns text into a fixed vector.
type Embedder interface {
	Name() string
	Dim() int
	Embed(ctx context.Context, text string) ([]float32, error)
}

// ModelMeta is written under data/models for init checks.
type ModelMeta struct {
	Name      string `json:"name"`
	Dim       int    `json:"dim"`
	SHA256    string `json:"sha256"`
	Algo      string `json:"algo"`
	ModelURL  string `json:"model_url,omitempty"`
	ORTReady  bool   `json:"ort_ready"`
	ONNXReady bool   `json:"onnx_ready"`
}

// EnsureModel downloads MiniLM ONNX + vocab + ORT from the network (explicit only),
// verifies checksums, and writes embedder.json. Falls back to hash meta if download fails
// only when allowFallback is true via EnsureModelWithOptions.
func EnsureModel(modelsDir string) (bool, error) {
	return EnsureModelWithOptions(modelsDir, true)
}

// EnsureModelWithOptions controls fallback.
func EnsureModelWithOptions(modelsDir string, allowFallback bool) (bool, error) {
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		return false, err
	}

	onnxOK := false
	ortOK := false

	// 1) model + vocab from Hugging Face
	if err := DownloadFile(DefaultModelURL, ModelONNXPath(modelsDir), DefaultModelSHA256); err != nil {
		if !allowFallback {
			return false, fmt.Errorf("minilm onnx download: %w", err)
		}
	} else {
		onnxOK = true
	}
	if err := DownloadFile(DefaultVocabURL, VocabPath(modelsDir), ""); err != nil {
		if !allowFallback {
			return false, fmt.Errorf("vocab download: %w", err)
		}
		onnxOK = false
	}

	// 2) ONNX Runtime shared lib
	if _, err := EnsureORTLib(modelsDir); err != nil {
		if !allowFallback {
			return false, fmt.Errorf("onnxruntime download: %w", err)
		}
	} else {
		ortOK = true
	}

	algo := "onnx-minilm-q"
	name := ONNXModelName
	sum := DefaultModelSHA256
	if !onnxOK || !ortOK {
		// hash fallback identity
		algo = "hash-ngram-v1"
		name = HashModelName
		sum = hashIdentitySHA()
		// still write hash-ready meta so product works offline after failed download
		_ = writeHashReady(modelsDir)
	}

	meta := ModelMeta{
		Name:      name,
		Dim:       Dim,
		SHA256:    sum,
		Algo:      algo,
		ModelURL:  DefaultModelURL,
		ORTReady:  ortOK,
		ONNXReady: onnxOK && ortOK,
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return false, err
	}
	path := filepath.Join(modelsDir, "embedder.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return false, err
	}
	return meta.ONNXReady || name == HashModelName, nil
}

func hashIdentitySHA() string {
	sum := sha256.Sum256([]byte(HashModelName + "|hash-ngram|dim=384"))
	return hex.EncodeToString(sum[:])
}

func writeHashReady(modelsDir string) error {
	// nothing else required for hash
	return nil
}

// ModelReady reports if embedder.json exists and is valid.
func ModelReady(modelsDir string) bool {
	path := filepath.Join(modelsDir, "embedder.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var got ModelMeta
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	if got.Dim != Dim {
		return false
	}
	if got.ONNXReady {
		if _, err := os.Stat(ModelONNXPath(modelsDir)); err != nil {
			return false
		}
		if _, err := os.Stat(VocabPath(modelsDir)); err != nil {
			return false
		}
		return true
	}
	return got.Name == HashModelName && got.SHA256 == hashIdentitySHA()
}

// ONNXReady is true when MiniLM ONNX assets are present.
func ONNXReady(modelsDir string) bool {
	if _, err := os.Stat(ModelONNXPath(modelsDir)); err != nil {
		return false
	}
	if _, err := os.Stat(VocabPath(modelsDir)); err != nil {
		return false
	}
	if _, err := EnsureORTLib(modelsDir); err != nil {
		return false
	}
	return true
}

// Open returns the best available embedder (ONNX MiniLM preferred).
func Open(modelsDir string) (Embedder, error) {
	if ONNXReady(modelsDir) {
		m, err := NewMiniLMONNX(modelsDir)
		if err == nil {
			return m, nil
		}
		// fall through to hash if construct fails
	}
	return HashEmbedder{}, nil
}

// ActiveModelName returns which model id will be used for storage.
func ActiveModelName(modelsDir string) string {
	e, err := Open(modelsDir)
	if err != nil {
		return HashModelName
	}
	return e.Name()
}

// ModelsDir joins dataDir/models.
func ModelsDir(dataDir string) string {
	return filepath.Join(dataDir, "models")
}

// ExpectedSHA256 kept for older call sites (hash identity).
func ExpectedSHA256() string { return hashIdentitySHA() }

// Float32ToBytes packs little-endian floats.
func Float32ToBytes(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// BytesToFloat32 unpacks little-endian floats.
func BytesToFloat32(b []byte) []float32 {
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

// SaveEmbedding writes/replaces an embedding row.
func SaveEmbedding(ctx context.Context, db *sql.DB, refType, refID, model string, vec []float32) error {
	id := fmt.Sprintf("%s:%s:%s", refType, refID, model)
	_, err := db.ExecContext(ctx, `
		INSERT INTO embeddings(id, ref_type, ref_id, model, dim, vector, created_at)
		VALUES (?,?,?,?,?,?,strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(ref_type, ref_id, model) DO UPDATE SET
			vector = excluded.vector,
			dim = excluded.dim,
			created_at = excluded.created_at`,
		id, refType, refID, model, len(vec), Float32ToBytes(vec),
	)
	return err
}

// EnsureReady downloads if needed and requires model ready.
func EnsureReady(modelsDir string) error {
	ok, err := EnsureModel(modelsDir)
	if err != nil {
		return err
	}
	if !ok || !ModelReady(modelsDir) {
		return fmt.Errorf("embed model not ready in %s", modelsDir)
	}
	return nil
}
