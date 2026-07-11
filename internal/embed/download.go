package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Hugging Face + Microsoft download targets (pinned).
const (
	// Quantized MiniLM ONNX (~23MB) from Xenova export of all-MiniLM-L6-v2.
	DefaultModelURL = "https://huggingface.co/Xenova/all-MiniLM-L6-v2/resolve/main/onnx/model_quantized.onnx"
	// Known content sha256 of model_quantized.onnx (LFS oid).
	DefaultModelSHA256 = "afdb6f1a0e45b715d0bb9b11772f032c399babd23bfc31fed1c170afc848bdb1"

	DefaultVocabURL = "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"

	// ONNX Runtime shared library (must support onnxruntime_go API).
	DefaultORTVersion = "1.27.1"
)

// Asset paths under modelsDir.
func ModelONNXPath(modelsDir string) string {
	return filepath.Join(modelsDir, "minilm", "model_quantized.onnx")
}
func VocabPath(modelsDir string) string {
	return filepath.Join(modelsDir, "minilm", "vocab.txt")
}
func ORTLibPath(modelsDir string) string {
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(modelsDir, "ort", "libonnxruntime.so")
	case "darwin":
		return filepath.Join(modelsDir, "ort", "libonnxruntime.dylib")
	default:
		return filepath.Join(modelsDir, "ort", "onnxruntime.dll")
	}
}

// DownloadFile fetches url to dest, optionally verifying sha256 hex.
func DownloadFile(url, dest, wantSHA string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	// reuse if checksum matches
	if wantSHA != "" {
		if sum, err := fileSHA256(dest); err == nil && sum == wantSHA {
			return nil
		}
	} else if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return nil
	}

	tmp := dest + ".partial"
	_ = os.Remove(tmp)

	client := &http.Client{Timeout: 15 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "orma-embed/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %s", url, resp.Status)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	if _, err := io.Copy(w, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" && sum != wantSHA {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", dest, sum, wantSHA)
	}
	return os.Rename(tmp, dest)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ORTDownloadURL returns the platform archive URL for ONNX Runtime.
func ORTDownloadURL() (url, memberLib string, err error) {
	ver := DefaultORTVersion
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-linux-x64-%s.tgz", ver, ver),
			fmt.Sprintf("onnxruntime-linux-x64-%s/lib/libonnxruntime.so.%s", ver, ver), nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-linux-aarch64-%s.tgz", ver, ver),
			fmt.Sprintf("onnxruntime-linux-aarch64-%s/lib/libonnxruntime.so.%s", ver, ver), nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-osx-x86_64-%s.tgz", ver, ver),
			fmt.Sprintf("onnxruntime-osx-x86_64-%s/lib/libonnxruntime.%s.dylib", ver, ver), nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return fmt.Sprintf("https://github.com/microsoft/onnxruntime/releases/download/v%s/onnxruntime-osx-arm64-%s.tgz", ver, ver),
			fmt.Sprintf("onnxruntime-osx-arm64-%s/lib/libonnxruntime.%s.dylib", ver, ver), nil
	default:
		return "", "", fmt.Errorf("unsupported platform %s/%s for onnxruntime auto-download", runtime.GOOS, runtime.GOARCH)
	}
}
