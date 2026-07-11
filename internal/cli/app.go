package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/distill"
	"github.com/anandh8x/orma/internal/embed"
	"github.com/anandh8x/orma/internal/recall"
	"github.com/anandh8x/orma/internal/runstate"
	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
)

type app struct {
	cfg *config.Config
	st  *store.Store
}

func openApp() (*app, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsureDataDir(); err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
	if err != nil {
		return nil, err
	}
	return &app{cfg: cfg, st: st}, nil
}

func (a *app) close() {
	if a.st != nil {
		_ = a.st.Close()
	}
}

func (a *app) wf() *workflow.Service   { return &workflow.Service{Store: a.st} }
func (a *app) distill() *distill.Service {
	return &distill.Service{Store: a.st, Workflow: a.wf()}
}
func (a *app) runs() *runstate.Service { return &runstate.Service{Store: a.st} }
func (a *app) recall() *recall.Service {
	var emb embed.Embedder
	if embed.ModelReady(embed.ModelsDir(a.cfg.DataDir)) {
		emb = embed.HashEmbedder{}
	}
	return &recall.Service{Store: a.st, Embed: emb, Model: embed.ModelName}
}

func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return "orma"
	}
	return p
}

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		return ""
	}
	return d
}

func die(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w", err)
}
