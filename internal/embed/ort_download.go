package embed

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureORTLib downloads ONNX Runtime shared library into modelsDir/ort if missing.
func EnsureORTLib(modelsDir string) (string, error) {
	dest := ORTLibPath(modelsDir)
	if st, err := os.Stat(dest); err == nil && st.Size() > 1000 {
		return dest, nil
	}
	// also accept versioned file next to dest
	dir := filepath.Dir(dest)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.Contains(name, "onnxruntime") && (strings.HasSuffix(name, ".so") || strings.Contains(name, ".so.") || strings.HasSuffix(name, ".dylib")) {
				p := filepath.Join(dir, name)
				if st, err := os.Stat(p); err == nil && st.Size() > 1000 {
					return p, nil
				}
			}
		}
	}

	url, member, err := ORTDownloadURL()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmpTGZ := filepath.Join(dir, "ort.tgz")
	client := &http.Client{Timeout: 15 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "orma-embed/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ort download: HTTP %s", resp.Status)
	}
	f, err := os.Create(tmpTGZ)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return "", err
	}
	_ = f.Close()
	defer os.Remove(tmpTGZ)

	extracted, err := extractTarMember(tmpTGZ, member, dir)
	if err != nil {
		// try to find any libonnxruntime in archive
		extracted, err = extractTarGlob(tmpTGZ, dir)
		if err != nil {
			return "", err
		}
	}
	// copy/link to stable dest name
	if extracted != dest {
		_ = os.Remove(dest)
		if err := copyFile(extracted, dest); err != nil {
			// use extracted path directly
			return extracted, nil
		}
	}
	_ = os.Chmod(dest, 0o755)
	return dest, nil
}

func extractTarMember(tgz, member, destDir string) (string, error) {
	f, err := os.Open(tgz)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Name != member && !strings.HasSuffix(hdr.Name, filepath.Base(member)) {
			continue
		}
		out := filepath.Join(destDir, filepath.Base(hdr.Name))
		w, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(w, tr); err != nil {
			_ = w.Close()
			return "", err
		}
		_ = w.Close()
		return out, nil
	}
	return "", fmt.Errorf("member %s not found in archive", member)
}

func extractTarGlob(tgz, destDir string) (string, error) {
	f, err := os.Open(tgz)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var last string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		base := filepath.Base(hdr.Name)
		if !strings.Contains(base, "libonnxruntime") {
			continue
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		out := filepath.Join(destDir, base)
		w, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(w, tr); err != nil {
			_ = w.Close()
			return "", err
		}
		_ = w.Close()
		last = out
	}
	if last == "" {
		return "", fmt.Errorf("no libonnxruntime in archive")
	}
	return last, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
