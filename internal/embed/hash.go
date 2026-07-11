package embed

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// HashModelName is the offline fallback embedder id.
const HashModelName = "minilm-hash-v1"

// HashEmbedder is a local offline embedder (no ONNX). Same dim as MiniLM.
type HashEmbedder struct{}

func (HashEmbedder) Name() string { return HashModelName }
func (HashEmbedder) Dim() int     { return Dim }

func (HashEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	vec := make([]float32, Dim)
	toks := hashTokens(text)
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
	l2Normalize(vec)
	return vec, nil
}

func hashTokens(s string) []string {
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

func l2Normalize(vec []float32) {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm <= 0 {
		return
	}
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
}
