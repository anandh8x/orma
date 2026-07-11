package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
	"github.com/spf13/cobra"
)

func newIngestCmd() *cobra.Command {
	var (
		jsonOne bool
		jsonl   bool
	)
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest one event (JSON) or many (JSONL) from stdin",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !jsonOne && !jsonl {
				// default: single JSON object
				jsonOne = true
			}
			if jsonOne && jsonl {
				return fmt.Errorf("use only one of --json or --jsonl")
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.EnsureDataDir(); err != nil {
				return err
			}
			st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
			if err != nil {
				return err
			}
			defer st.Close()

			svc := &ingest.Service{Store: st, Config: cfg}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if jsonl {
				return ingestJSONL(ctx, svc, os.Stdin)
			}
			return ingestJSON(ctx, svc, os.Stdin)
		},
	}
	cmd.Flags().BoolVar(&jsonOne, "json", false, "read one JSON event object from stdin (default)")
	cmd.Flags().BoolVar(&jsonl, "jsonl", false, "read JSONL events from stdin")
	return cmd
}

func ingestJSON(ctx context.Context, svc *ingest.Service, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty stdin")
	}
	res, err := svc.IngestOne(ctx, data)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(res)
}

func ingestJSONL(ctx context.Context, svc *ingest.Service, r io.Reader) error {
	sc := bufio.NewScanner(r)
	// allow long lines for commands
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	var n int
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		res, err := svc.IngestOne(ctx, line)
		if err != nil {
			return fmt.Errorf("line %d: %w", n+1, err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(res); err != nil {
			return err
		}
		n++
	}
	return sc.Err()
}

func bytesTrimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\t' || b[j-1] == '\r' || b[j-1] == '\n') {
		j--
	}
	return b[i:j]
}
