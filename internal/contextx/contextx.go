package contextx

import (
	"context"
	"fmt"
	"strings"

	"github.com/anandh8x/orma/internal/project"
	"github.com/anandh8x/orma/internal/recall"
	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
)

// Service builds agent-facing runbook text from memory.
type Service struct {
	Store  *store.Store
	Recall *recall.Service
	WF     *workflow.Service
}

// Build returns markdown a coding agent can follow.
func (s *Service) Build(ctx context.Context, query, cwd string, limit int) (string, error) {
	if limit <= 0 {
		limit = 5
	}
	proj := project.Resolve(cwd)
	hits, err := s.Recall.Query(ctx, query, proj, limit, false)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# Orma context\n\n")
	if query != "" {
		b.WriteString("Query: ")
		b.WriteString(query)
		b.WriteString("\n\n")
	}
	if proj != "" {
		b.WriteString("Project: `")
		b.WriteString(proj)
		b.WriteString("`\n\n")
	}
	if len(hits) == 0 {
		b.WriteString("No matching workflows or notes.\n")
		return b.String(), nil
	}

	b.WriteString("Use these proven local steps when relevant. Prefer successful paths. Adapt paths to the current machine.\n\n")

	for i, h := range hits {
		title := h.Title
		if title == "" {
			title = h.RefID
		}
		fmt.Fprintf(&b, "## %d. %s (%s)\n\n", i+1, title, h.RefType)
		if h.Origin != "" {
			fmt.Fprintf(&b, "Origin: %s\n\n", h.Origin)
		}
		if h.RefType == "workflow" {
			w, err := s.WF.Get(ctx, h.RefID)
			if err == nil && len(w.Steps) > 0 {
				b.WriteString("```bash\n")
				for _, st := range w.Steps {
					b.WriteString(st.Command)
					if st.Outcome != "" && st.Outcome != "unknown" {
						b.WriteString("  # ")
						b.WriteString(st.Outcome)
					}
					b.WriteByte('\n')
				}
				b.WriteString("```\n\n")
				continue
			}
		}
		if strings.TrimSpace(h.Body) != "" {
			b.WriteString(h.Body)
			b.WriteString("\n\n")
		}
	}
	return b.String(), nil
}
