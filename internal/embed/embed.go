package embed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// ModelName is the local embedder id (hashing n-gram, MiniLM-compatible dim).
	ModelName = "minilm-hash-v1"
	// Dim matches common MiniLM embedding size for drop-in later.
	Dim = 384
)

// Embedder turns text into a fixed vector.
type Embedder interface {
	Name() string
	Dim() int
	Embed(ctx context.Context, text string) ([]float32, error)
}

// HashEmbedder is a local offline embedder (no ONNX runtime required).
// Same dim as MiniLM so the store/daemon path matches the plan.
type HashEmbedder struct{}

func (HashEmbedder) Name() string { return ModelName }
func (HashEmbedder) Dim() int     { return Dim }

func (HashEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	vec := make([]float32, Dim)
	toks := tokens(text)
	if len(toks) == 0 {
		return vec, nil
	}
	for _, t := range toks {
		h := fnv.New32a()
		_, _ = h.Write([]byte(t))
		idx := int(h.Sum32() % uint32(Dim))
		vec[idx] += 1
		h2 := fnv.New32a()
		_, _ = h2.Write([]byte("s:" + t))
		if h2.Sum32()%2 == 0 {
			vec[idx] += 0.25
		} else {
			vec[idx] -= 0.25
		}
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}
	return vec, nil
}

func tokens(s string) []string {
	s = strings.ToLower(s)
	var b strings.Builder
	var out []string
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// ModelMeta is written under data/models for init checks.
type ModelMeta struct {
	Name   string `json:"name"`
	Dim    int    `json:"dim"`
	SHA256 string `json:"sha256"`
	Algo   string `json:"algo"`
}

// ExpectedSHA256 is the checksum of the canonical embedder identity.
func ExpectedSHA256() string {
	sum := sha256.Sum256([]byte(ModelName + "|hash-ngram|dim=384"))
	return hex.EncodeToString(sum[:])
}

// EnsureModel writes models/embedder.json if missing or stale.
func EnsureModel(modelsDir string) (bool, error) {
	if err := os.MkdirAll(modelsDir, 0o700); err != nil {
		return false, err
	}
	path := filepath.Join(modelsDir, "embedder.json")
	meta := ModelMeta{
		Name:   ModelName,
		Dim:    Dim,
		SHA256: ExpectedSHA256(),
		Algo:   "hash-ngram-v1",
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return false, err
	}
	if existing, err := os.ReadFile(path); err == nil {
		var got ModelMeta
		if json.Unmarshal(existing, &got) == nil && got.SHA256 == ExpectedSHA256() && got.Name == ModelName {
			return true, nil
		}
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// ModelReady reports if embedder.json is present and checksum matches.
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
	return got.SHA256 == ExpectedSHA256() && got.Name == ModelName
}

// ModelsDir joins dataDir/models.
func ModelsDir(dataDir string) string {
	return filepath.Join(dataDir, "models")
}

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
