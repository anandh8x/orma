package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/store"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create config and database (model download comes later)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.EnsureDataDir(); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return err
			}

			st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
			if err != nil {
				return err
			}
			defer st.Close()

			// Placeholder for future model step: init is "partial" until embed model exists.
			modelsDir := filepath.Join(cfg.DataDir, "models")
			if err := os.MkdirAll(modelsDir, 0o700); err != nil {
				return err
			}
			modelMarker := filepath.Join(modelsDir, ".required")
			if _, err := os.Stat(filepath.Join(modelsDir, "minilm.onnx")); err != nil {
				_ = os.WriteFile(modelMarker, []byte("minilm model not installed yet\n"), 0o600)
				fmt.Printf("config: %s\n", cfg.Path())
				fmt.Printf("db: %s\n", cfg.DBPath())
				fmt.Println("init: store ready")
				fmt.Println("init: embedding model not installed yet (required for full init later)")
				return nil
			}

			fmt.Printf("config: %s\n", cfg.Path())
			fmt.Printf("db: %s\n", cfg.DBPath())
			fmt.Println("init: ok")
			return nil
		},
	}
}
