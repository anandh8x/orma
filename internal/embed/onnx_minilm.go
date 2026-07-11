package embed

import (
	"context"
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// ONNXModelName is the stored embeddings.model value for MiniLM ONNX.
const ONNXModelName = "all-MiniLM-L6-v2-onnx-q"

// MiniLM ONNX session embedder (Hugging Face quantized model).
type MiniLMONNX struct {
	modelPath string
	libPath   string
	tok       *BertWordPiece
	mu        sync.Mutex
}

var (
	ortOnce sync.Once
	ortErr  error
	ortLib  string
)

func initORT(libPath string) error {
	ortOnce.Do(func() {
		ortLib = libPath
		ort.SetSharedLibraryPath(libPath)
		ortErr = ort.InitializeEnvironment()
	})
	if ortErr != nil {
		return ortErr
	}
	// if a second path is requested and differs, still use first successful init
	_ = ortLib
	return nil
}

// NewMiniLMONNX builds an embedder. Call EnsureModel first.
func NewMiniLMONNX(modelsDir string) (*MiniLMONNX, error) {
	tok, err := LoadVocab(VocabPath(modelsDir), 128)
	if err != nil {
		return nil, fmt.Errorf("vocab: %w", err)
	}
	lib := ORTLibPath(modelsDir)
	if p, err := EnsureORTLib(modelsDir); err == nil {
		lib = p
	}
	return &MiniLMONNX{
		modelPath: ModelONNXPath(modelsDir),
		libPath:   lib,
		tok:       tok,
	}, nil
}

func (m *MiniLMONNX) Name() string { return ONNXModelName }
func (m *MiniLMONNX) Dim() int     { return Dim }

func (m *MiniLMONNX) ensureEnv() error {
	return initORT(m.libPath)
}

// Embed runs MiniLM and returns L2-normalized mean-pooled 384-d vector.
func (m *MiniLMONNX) Embed(ctx context.Context, text string) ([]float32, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureEnv(); err != nil {
		return nil, err
	}

	ids, mask, types := m.tok.Encode(text)
	seq := int64(len(ids))
	if seq == 0 {
		return make([]float32, Dim), nil
	}

	shape := ort.NewShape(1, seq)
	idT, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, err
	}
	defer idT.Destroy()
	maskT, err := ort.NewTensor(shape, mask)
	if err != nil {
		return nil, err
	}
	defer maskT.Destroy()
	typeT, err := ort.NewTensor(shape, types)
	if err != nil {
		return nil, err
	}
	defer typeT.Destroy()

	// output: [1, seq, 384]
	outShape := ort.NewShape(1, seq, int64(Dim))
	outT, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return nil, err
	}
	defer outT.Destroy()

	session, err := ort.NewAdvancedSession(
		m.modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{idT, maskT, typeT},
		[]ort.Value{outT},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("onnx session: %w", err)
	}
	defer session.Destroy()

	if err := session.Run(); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}

	data := outT.GetData() // len = seq * 384
	vec := meanPool(data, mask, int(seq), Dim)
	l2Normalize(vec)
	return vec, nil
}

func meanPool(hidden []float32, mask []int64, seq, dim int) []float32 {
	out := make([]float32, dim)
	var count float32
	for i := 0; i < seq; i++ {
		if mask[i] == 0 {
			continue
		}
		count++
		base := i * dim
		for d := 0; d < dim; d++ {
			out[d] += hidden[base+d]
		}
	}
	if count == 0 {
		return out
	}
	for d := range out {
		out[d] /= count
	}
	return out
}
