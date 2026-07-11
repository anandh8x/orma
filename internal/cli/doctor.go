package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/store"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local config and database",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			fmt.Printf("config: %s\n", cfg.Path())
			if _, err := os.Stat(cfg.Path()); err != nil {
				fmt.Println("  (file missing, defaults in use until save/init)")
			} else {
				fmt.Println("  present")
			}

			fmt.Printf("data_dir: %s\n", cfg.DataDir)
			fmt.Printf("db: %s\n", cfg.DBPath())
			fmt.Printf("redact: %v\n", cfg.Redact)
			fmt.Printf("session_idle: %s\n", cfg.SessionIdle.Duration)

			if err := cfg.EnsureDataDir(); err != nil {
				return err
			}

			st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer st.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := st.Ping(ctx); err != nil {
				return err
			}
			ver, err := st.SchemaVersion()
			if err != nil {
				return err
			}
			fmt.Printf("schema_version: %d\n", ver)

			var events, sessions int
			_ = st.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events)
			_ = st.DB().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
			fmt.Printf("events: %d\n", events)
			fmt.Printf("sessions: %d\n", sessions)
			fmt.Println("doctor: ok")
			return nil
		},
	}
}
