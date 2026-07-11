package recall

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"

	"github.com/anandh8x/orma/internal/embed"
	"github.com/anandh8x/orma/internal/store"
)

// Hit is one recall result.
type Hit struct {
	RefType     string
	RefID       string
	Title       string
	Body        string
	ProjectRoot string
	Score       float64
	Pinned      bool
	Origin      string
}

// Service searches memory.
type Service struct {
	Store *store.Store
	Embed embed.Embedder
	Model string
}

// Query runs keyword search (+ optional vectors), then ranks.
func (s *Service) Query(ctx context.Context, q, projectRoot string, limit int, raw bool) ([]Hit, error) {
	if limit <= 0 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return s.recent(ctx, projectRoot, limit)
	}

	hits, err := s.likeSearch(ctx, q, projectRoot, limit*2)
	if err != nil {
		return nil, err
	}

	// best-effort FTS enrichment
	if fts, err := s.ftsSearch(ctx, q, limit*2); err == nil {
		hits = mergeHits(hits, fts)
	}

	if s.Embed != nil {
		if vecHits, err := s.vectorSearch(ctx, q, limit*2); err == nil {
			hits = mergeHits(hits, vecHits)
		}
	}

	if !raw {
		hits = filterDefault(hits)
	}
	rankHits(hits, projectRoot)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (s *Service) likeSearch(ctx context.Context, q, projectRoot string, limit int) ([]Hit, error) {
	like := "%" + q + "%"
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id, COALESCE(name,''), COALESCE(project_root,''), COALESCE(body,''), pinned, COALESCE(origin,'')
		FROM workflows
		WHERE (name LIKE ? OR body LIKE ? OR id LIKE ?)
		ORDER BY pinned DESC, updated_at DESC
		LIMIT ?`, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var pinned int
		if err := rows.Scan(&h.RefID, &h.Title, &h.ProjectRoot, &h.Body, &pinned, &h.Origin); err != nil {
			return nil, err
		}
		h.RefType = "workflow"
		h.Pinned = pinned == 1
		h.Score = 3
		if projectRoot != "" && h.ProjectRoot == projectRoot {
			h.Score += 1
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// notes
	nrows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id, text, COALESCE(project_root,'') FROM notes
		WHERE text LIKE ? ORDER BY created_at DESC LIMIT ?`, like, limit)
	if err == nil {
		defer nrows.Close()
		for nrows.Next() {
			var h Hit
			if err := nrows.Scan(&h.RefID, &h.Body, &h.ProjectRoot); err != nil {
				break
			}
			h.RefType = "note"
			h.Title = "note"
			h.Score = 2
			hits = append(hits, h)
		}
	}

	// fixes
	frows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id, error_fingerprint, COALESCE(resolution_workflow_id,'') FROM fixes
		WHERE error_fingerprint LIKE ? ORDER BY updated_at DESC LIMIT ?`, like, limit)
	if err == nil {
		defer frows.Close()
		for frows.Next() {
			var h Hit
			var wf string
			if err := frows.Scan(&h.RefID, &h.Body, &wf); err != nil {
				break
			}
			h.RefType = "fix"
			h.Title = "fix"
			h.Score = 2.5
			hits = append(hits, h)
		}
	}
	return hits, nil
}

func (s *Service) ftsSearch(ctx context.Context, q string, limit int) ([]Hit, error) {
	ftsQuery := toFTSQuery(q)
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT ref_type, ref_id, COALESCE(project_root,''), COALESCE(title,''), body
		FROM memory_fts
		WHERE memory_fts MATCH ?
		LIMIT ?`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.RefType, &h.RefID, &h.ProjectRoot, &h.Title, &h.Body); err != nil {
			return nil, err
		}
		h.Score = 4
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range hits {
		s.enrich(ctx, &hits[i])
	}
	return hits, nil
}

func (s *Service) recent(ctx context.Context, projectRoot string, limit int) ([]Hit, error) {
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT id, COALESCE(name,''), COALESCE(project_root,''), COALESCE(body,''), pinned, COALESCE(origin,'')
		FROM workflows
		WHERE ? = '' OR project_root = ? OR project_root IS NULL
		ORDER BY pinned DESC, updated_at DESC
		LIMIT ?`, projectRoot, projectRoot, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var pinned int
		if err := rows.Scan(&h.RefID, &h.Title, &h.ProjectRoot, &h.Body, &pinned, &h.Origin); err != nil {
			return nil, err
		}
		h.RefType = "workflow"
		h.Pinned = pinned == 1
		h.Score = 1
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (s *Service) vectorSearch(ctx context.Context, q string, limit int) ([]Hit, error) {
	qv, err := s.Embed.Embed(ctx, q)
	if err != nil {
		return nil, err
	}
	model := s.Model
	if model == "" {
		model = s.Embed.Name()
	}
	rows, err := s.Store.DB().QueryContext(ctx, `
		SELECT ref_type, ref_id, vector FROM embeddings WHERE model = ? LIMIT 2000`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		h Hit
		s float64
	}
	var all []scored
	for rows.Next() {
		var refType, refID string
		var blob []byte
		if err := rows.Scan(&refType, &refID, &blob); err != nil {
			return nil, err
		}
		vec := embed.BytesToFloat32(blob)
		sim := cosine(qv, vec)
		all = append(all, scored{h: Hit{RefType: refType, RefID: refID, Score: sim * 5}, s: sim})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
	if len(all) > limit {
		all = all[:limit]
	}
	var hits []Hit
	for _, a := range all {
		hits = append(hits, a.h)
	}
	for i := range hits {
		s.enrich(ctx, &hits[i])
	}
	return hits, nil
}

func (s *Service) enrich(ctx context.Context, h *Hit) {
	switch h.RefType {
	case "workflow":
		var name, project, body, origin sql.NullString
		var pinned int
		_ = s.Store.DB().QueryRowContext(ctx, `
			SELECT name, project_root, body, pinned, origin FROM workflows WHERE id = ?`, h.RefID,
		).Scan(&name, &project, &body, &pinned, &origin)
		if h.Title == "" {
			h.Title = name.String
		}
		if h.Body == "" {
			h.Body = body.String
		}
		if h.ProjectRoot == "" {
			h.ProjectRoot = project.String
		}
		h.Pinned = pinned == 1
		h.Origin = origin.String
	case "note":
		var text, project sql.NullString
		_ = s.Store.DB().QueryRowContext(ctx, `
			SELECT text, project_root FROM notes WHERE id = ?`, h.RefID,
		).Scan(&text, &project)
		h.Title = "note"
		h.Body = text.String
		h.ProjectRoot = project.String
	case "fix":
		var fp sql.NullString
		_ = s.Store.DB().QueryRowContext(ctx, `
			SELECT error_fingerprint FROM fixes WHERE id = ?`, h.RefID).Scan(&fp)
		h.Title = "fix"
		h.Body = fp.String
	}
	var n int
	_ = s.Store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pins WHERE ref_type = ? AND ref_id = ?`, h.RefType, h.RefID).Scan(&n)
	if n > 0 {
		h.Pinned = true
	}
}

func rankHits(hits []Hit, projectRoot string) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		aProj := projectRoot != "" && a.ProjectRoot == projectRoot
		bProj := projectRoot != "" && b.ProjectRoot == projectRoot
		if aProj != bProj {
			return aProj
		}
		if a.Score == b.Score && a.Origin != b.Origin {
			return a.Origin != "agent"
		}
		return a.Score > b.Score
	})
}

func filterDefault(hits []Hit) []Hit {
	var out []Hit
	for _, h := range hits {
		if h.RefType == "workflow" || h.RefType == "note" || h.RefType == "fix" {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return hits
	}
	return out
}

func mergeHits(a, b []Hit) []Hit {
	m := map[string]Hit{}
	key := func(h Hit) string { return h.RefType + ":" + h.RefID }
	for _, h := range a {
		m[key(h)] = h
	}
	for _, h := range b {
		if old, ok := m[key(h)]; ok {
			old.Score = (old.Score + h.Score) / 2
			m[key(h)] = old
		} else {
			m[key(h)] = h
		}
	}
	out := make([]Hit, 0, len(m))
	for _, h := range m {
		out = append(out, h)
	}
	return out
}

func toFTSQuery(q string) string {
	parts := strings.Fields(q)
	for i, p := range parts {
		p = strings.ReplaceAll(p, `"`, "")
		p = strings.ReplaceAll(p, "'", "")
		parts[i] = `"` + p + `"`
	}
	return strings.Join(parts, " OR ")
}

func cosine(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
