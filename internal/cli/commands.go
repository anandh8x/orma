package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"io"

	"github.com/anandh8x/orma/internal/adapt"
	"github.com/anandh8x/orma/internal/adapters/claude"
	"github.com/anandh8x/orma/internal/adapters/codex"
	"github.com/anandh8x/orma/internal/adapters/opencode"
	"github.com/anandh8x/orma/internal/clipboard"
	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/contextx"
	"github.com/anandh8x/orma/internal/daemon"
	"github.com/anandh8x/orma/internal/embed"
	"github.com/anandh8x/orma/internal/event"
	"github.com/anandh8x/orma/internal/fix"
	"github.com/anandh8x/orma/internal/history"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/picker"
	"github.com/anandh8x/orma/internal/project"
	"github.com/anandh8x/orma/internal/recall"
	"github.com/anandh8x/orma/internal/runexec"
	"github.com/anandh8x/orma/internal/shellembed"
	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
	"github.com/spf13/cobra"
)

func registerAll(root *cobra.Command) {
	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newHookCmd())
	root.AddCommand(newHookCaptureCmd())
	root.AddCommand(newHookExitCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(newSaveCmd())
	root.AddCommand(newNoteCmd())
	root.AddCommand(newPinCmd())
	root.AddCommand(newHereCmd())
	root.AddCommand(newSessionsCmd())
	root.AddCommand(newLastCmd())
	root.AddCommand(newRecallCmd())
	root.AddCommand(newContextCmd())
	root.AddCommand(newUseCmd())
	root.AddCommand(newNextCmd())
	root.AddCommand(newDistillCmd())
	root.AddCommand(newDaemonCmd())
	root.AddCommand(newConnectCmd())
	root.AddCommand(newPurgeCmd())
	root.AddCommand(newEmbedCmd())
	root.AddCommand(newWorkflowCmd())
	root.AddCommand(newFixCmd())
}

func newInitCmd() *cobra.Command {
	var skipHist bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create config, database, embedder model; optional history import",
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

			ok, err := embed.EnsureModel(embed.ModelsDir(cfg.DataDir))
			if err != nil {
				return err
			}
			if !ok || !embed.ModelReady(embed.ModelsDir(cfg.DataDir)) {
				return fmt.Errorf("embedder model not ready under %s", embed.ModelsDir(cfg.DataDir))
			}

			fmt.Printf("config: %s\n", cfg.Path())
			fmt.Printf("db: %s\n", cfg.DBPath())
			fmt.Printf("model: %s ready\n", embed.ModelName)

			if !skipHist {
				n, _ := history.CountHistLines()
				if n > 0 {
					fmt.Printf("import %d history lines? [y/N] ", n)
					sc := bufio.NewScanner(os.Stdin)
					if sc.Scan() && strings.HasPrefix(strings.ToLower(strings.TrimSpace(sc.Text())), "y") {
						ctx, cancel := withTimeout()
						defer cancel()
						imported, err := history.ImportShellHist(ctx, st, cfg, 0)
						if err != nil {
							return err
						}
						fmt.Printf("imported %d events\n", imported)
					}
				}
			}

			fmt.Println("init: ok")
			fmt.Printf("binary: %s\n", exePath())
			fmt.Printf("embedder: %s (onnx=%v)\n", embed.ActiveModelName(embed.ModelsDir(cfg.DataDir)), embed.ONNXReady(embed.ModelsDir(cfg.DataDir)))
			fmt.Println("add one line to your shell rc, then open a new terminal:")
			fmt.Println(`  eval "$(orma hook zsh)"`)
			fmt.Println(`  # or: eval "$(orma hook bash)"`)
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipHist, "skip-history", false, "do not prompt for history import")
	return cmd
}

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
				fmt.Println("  (missing, defaults until init)")
			} else {
				fmt.Println("  present")
			}
			fmt.Printf("data_dir: %s\n", cfg.DataDir)
			fmt.Printf("db: %s\n", cfg.DBPath())
			fmt.Printf("redact: %v\n", cfg.Redact)
			fmt.Printf("session_idle: %s\n", cfg.SessionIdle.Duration)
			mdir := embed.ModelsDir(cfg.DataDir)
			fmt.Printf("model_ready: %v\n", embed.ModelReady(mdir))
			fmt.Printf("onnx: %v active=%s\n", embed.ONNXReady(mdir), embed.ActiveModelName(mdir))

			running, pid, _ := daemon.Status(cfg.DataDir)
			fmt.Printf("daemon: running=%v pid=%d\n", running, pid)

			if err := cfg.EnsureDataDir(); err != nil {
				return err
			}
			st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
			if err != nil {
				return err
			}
			defer st.Close()
			ver, err := st.SchemaVersion()
			if err != nil {
				return err
			}
			fmt.Printf("schema_version: %d\n", ver)
			var events, sessions, workflows int
			_ = st.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events)
			_ = st.DB().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions)
			_ = st.DB().QueryRow(`SELECT COUNT(*) FROM workflows`).Scan(&workflows)
			fmt.Printf("events: %d\n", events)
			fmt.Printf("sessions: %d\n", sessions)
			fmt.Printf("workflows: %d\n", workflows)

			// size warning
			if fi, err := os.Stat(cfg.DBPath()); err == nil && fi.Size() > 500*1024*1024 {
				fmt.Printf("warning: db size %.1f MB (use orma purge if needed)\n", float64(fi.Size())/(1024*1024))
			}
			fmt.Println("doctor: ok")
			return nil
		},
	}
}

func newIngestCmd() *cobra.Command {
	var jsonOne, jsonl bool
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest one JSON event or JSONL from stdin",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !jsonOne && !jsonl {
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
				sc := bufio.NewScanner(os.Stdin)
				buf := make([]byte, 0, 64*1024)
				sc.Buffer(buf, 1024*1024)
				lineNo := 0
				for sc.Scan() {
					line := strings.TrimSpace(sc.Text())
					if line == "" {
						continue
					}
					lineNo++
					res, err := svc.IngestOne(ctx, []byte(line))
					if err != nil {
						return fmt.Errorf("line %d: %w", lineNo, err)
					}
					_ = json.NewEncoder(os.Stdout).Encode(res)
				}
				return sc.Err()
			}
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			res, err := svc.IngestOne(ctx, data)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(res)
		},
	}
	cmd.Flags().BoolVar(&jsonOne, "json", false, "one JSON object (default)")
	cmd.Flags().BoolVar(&jsonl, "jsonl", false, "JSONL stream")
	return cmd
}

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hook [zsh|bash]",
		Short: "Print shell integration script (hooks use this binary path)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := shellembed.Script(args[0], exePath())
			if err != nil {
				return err
			}
			_, err = os.Stdout.WriteString(script)
			return err
		},
	}
}

func newHookCaptureCmd() *cobra.Command {
	var shell string
	var exitCode int
	var cwdFlag string
	cmd := &cobra.Command{
		Use:    "hook-capture",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readStdin()
			if err != nil {
				return nil // fail open
			}
			cmdStr := strings.TrimSpace(string(data))
			if cmdStr == "" {
				return nil
			}
			cfg, err := config.Load()
			if err != nil {
				return nil
			}
			_ = cfg.EnsureDataDir()
			// daemon auto-start
			if running, _, _ := daemon.Status(cfg.DataDir); !running {
				startDaemonBackground(cfg)
			}
			st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
			if err != nil {
				return nil
			}
			defer st.Close()
			if cwdFlag == "" {
				cwdFlag = cwd()
			}
			ev := map[string]any{
				"schema":  event.SchemaV1,
				"ts":      time.Now().UTC().Format(time.RFC3339Nano),
				"actor":   "human",
				"kind":    "shell",
				"source":  "shell",
				"command": cmdStr,
				"cwd":     cwdFlag,
				"shell":   shell,
				"exit_code": exitCode,
			}
			b, _ := json.Marshal(ev)
			svc := &ingest.Service{Store: st, Config: cfg}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = svc.IngestOne(ctx, b)
			return nil
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "zsh", "shell name")
	cmd.Flags().IntVar(&exitCode, "exit", 0, "exit code")
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", "working directory")
	return cmd
}

func newHookExitCmd() *cobra.Command {
	var exitCode int
	cmd := &cobra.Command{
		Use:    "hook-exit",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return nil
			}
			defer a.close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cur, err := a.runs().Current(ctx)
			if err != nil || cur == nil {
				return nil
			}
			_, _ = a.runs().RecordExit(ctx, exitCode)
			return nil
		},
	}
	cmd.Flags().IntVar(&exitCode, "exit", 0, "exit code")
	return cmd
}

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import [history|atuin]",
		Short: "Import shell history or Atuin DB into memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			switch args[0] {
			case "history":
				n, err := history.ImportShellHist(ctx, a.st, a.cfg, 0)
				if err != nil {
					return err
				}
				fmt.Printf("imported %d events\n", n)
			case "atuin":
				n, err := history.ImportAtuin(ctx, a.st, a.cfg, "", 0)
				if err != nil {
					return err
				}
				fmt.Printf("imported %d events from atuin\n", n)
			default:
				return fmt.Errorf("usage: orma import history|atuin")
			}
			return nil
		},
	}
}

func newSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save <name>",
		Short: "Save recent project commands as a named workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			proj := project.Resolve(cwd())
			w, err := a.wf().SaveNamed(ctx, args[0], proj, 30)
			if err != nil {
				return err
			}
			// flush embeddings so recall is useful immediately
			_, _ = embed.ProcessQueue(ctx, a.st.DB(), a.embedder(), 50)
			fmt.Printf("saved workflow %s (%d steps) id=%s\n", w.Name, len(w.Steps), w.ID)
			return nil
		},
	}
}

func newNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <text...>",
		Short: "Add a note to memory",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			id, err := a.wf().AddNote(ctx, strings.Join(args, " "), project.Resolve(cwd()))
			if err != nil {
				return err
			}
			fmt.Println(id)
			return nil
		},
	}
}

func newPinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pin <workflow-id>",
		Short: "Pin a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			return a.wf().Pin(ctx, "workflow", args[0])
		},
	}
}

func newHereCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "here",
		Short: "List rituals/workflows for this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			proj := project.Resolve(cwd())
			list, err := a.wf().ListByProject(ctx, proj, 50)
			if err != nil {
				return err
			}
			fmt.Printf("project: %s\n", proj)
			for _, w := range list {
				pin := ""
				if w.Pinned {
					pin = "*"
				}
				name := w.Name
				if name == "" {
					name = w.ID[:8]
				}
				fmt.Printf("%s%-20s %2d steps  %s  %s\n", pin, name, len(w.Steps), w.Origin, w.ID)
			}
			return nil
		},
	}
}

func newSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions",
		Short: "List recent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			rows, err := a.st.DB().Query(`
				SELECT id, actor_mix, COALESCE(agent,''), COALESCE(project_root,''), status, last_event_at
				FROM sessions ORDER BY last_event_at DESC LIMIT 30`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id, mix, agent, proj, status, last string
				if err := rows.Scan(&id, &mix, &agent, &proj, &status, &last); err != nil {
					return err
				}
				fmt.Printf("%s  %s  %s  %s  %s\n", last, mix, agent, status, id)
			}
			return rows.Err()
		},
	}
}

func newRecallCmd() *cobra.Command {
	var pick bool
	var raw bool
	cmd := &cobra.Command{
		Use:   "recall [query]",
		Short: "Search workflows, notes, fixes",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			q := strings.Join(args, " ")
			hits, err := a.recall().Query(ctx, q, project.Resolve(cwd()), 20, raw)
			if err != nil {
				return err
			}
			if pick {
				if len(hits) == 0 {
					return nil
				}
				items := make([]picker.Item, 0, len(hits))
				for _, h := range hits {
					title := h.Title
					if title == "" {
						title = h.RefID
					}
					detail := strings.Split(h.Body, "\n")[0]
					items = append(items, picker.Item{
						Label:  fmt.Sprintf("[%s] %s", h.RefType, title),
						Detail: detail,
						Value:  h.RefType + "\x1e" + h.RefID,
					})
				}
				val, err := picker.Run("orma recall", items)
				if err != nil || val == "" {
					// non-TTY fallback: numbered pick on stderr
					return numberedPick(ctx, a, hits)
				}
				parts := strings.SplitN(val, "\x1e", 2)
				if len(parts) != 2 {
					return nil
				}
				return emitPick(ctx, a, parts[0], parts[1])
			}
			for _, h := range hits {
				pin := " "
				if h.Pinned {
					pin = "*"
				}
				title := h.Title
				if title == "" {
					title = h.RefID
				}
				fmt.Printf("%s [%s] %-24s score=%.2f %s\n", pin, h.RefType, title, h.Score, h.RefID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&pick, "pick", false, "interactive pick; print command for shell insert")
	cmd.Flags().BoolVar(&raw, "raw", false, "include raw-ish hits")
	return cmd
}

func numberedPick(ctx context.Context, a *app, hits []recall.Hit) error {
	for i, h := range hits {
		title := h.Title
		if title == "" {
			title = h.RefID
		}
		fmt.Fprintf(os.Stderr, "%d) [%s] %s\n", i+1, h.RefType, title)
		if h.Body != "" {
			fmt.Fprintf(os.Stderr, "   %s\n", truncate(strings.Split(h.Body, "\n")[0], 80))
		}
	}
	fmt.Fprint(os.Stderr, "pick #: ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return nil
	}
	n, _ := strconv.Atoi(strings.TrimSpace(sc.Text()))
	if n < 1 || n > len(hits) {
		return nil
	}
	h := hits[n-1]
	return emitPick(ctx, a, h.RefType, h.RefID)
}

func emitPick(ctx context.Context, a *app, refType, refID string) error {
	if refType == "workflow" {
		w, err := a.wf().Get(ctx, refID)
		if err != nil {
			return err
		}
		if len(w.Steps) == 0 {
			return nil
		}
		_, _ = a.runs().Start(ctx, w.ID)
		r := adapt.Apply(w.Steps[0].Command, adapt.Options{
			OldProject: w.ProjectRoot,
			NewProject: project.Resolve(cwd()),
			CWD:        cwd(),
			Aliases:    a.cfg.Aliases,
		})
		fmt.Print(r.Adapted)
		return nil
	}
	if refType == "fix" {
		fs := &fix.Service{Store: a.st}
		rec, err := fs.Get(ctx, refID)
		if err != nil {
			return err
		}
		if rec.ResolutionWorkflowID != "" {
			return emitPick(ctx, a, "workflow", rec.ResolutionWorkflowID)
		}
		fmt.Print(rec.ErrorFingerprint)
		return nil
	}
	var body string
	switch refType {
	case "note":
		_ = a.st.DB().QueryRowContext(ctx, `SELECT text FROM notes WHERE id = ?`, refID).Scan(&body)
	default:
		body = refID
	}
	if body != "" {
		fmt.Print(strings.Split(body, "\n")[0])
	}
	return nil
}

func newLastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "last",
		Short: "Show the most recent session and its commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			var id, mix, agent, proj, status, last string
			err = a.st.DB().QueryRow(`
				SELECT id, actor_mix, COALESCE(agent,''), COALESCE(project_root,''), status, last_event_at
				FROM sessions ORDER BY last_event_at DESC LIMIT 1`).Scan(&id, &mix, &agent, &proj, &status, &last)
			if err != nil {
				return fmt.Errorf("no sessions yet")
			}
			fmt.Printf("session %s  %s  %s  %s  %s\n", id, mix, agent, status, last)
			if proj != "" {
				fmt.Printf("project %s\n", proj)
			}
			rows, err := a.st.DB().Query(`
				SELECT COALESCE(command,''), COALESCE(outcome,''), COALESCE(exit_code,-999)
				FROM events WHERE session_id = ? AND command IS NOT NULL
				ORDER BY ts ASC LIMIT 50`, id)
			if err != nil {
				return err
			}
			defer rows.Close()
			i := 1
			for rows.Next() {
				var cmd, outcome string
				var exit int
				if err := rows.Scan(&cmd, &outcome, &exit); err != nil {
					return err
				}
				mark := outcome
				if exit != -999 && mark == "" {
					if exit == 0 {
						mark = "ok"
					} else {
						mark = "fail"
					}
				}
				fmt.Printf("%2d. [%s] %s\n", i, mark, cmd)
				i++
			}
			return rows.Err()
		},
	}
}

func newContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context [query]",
		Short: "Print a markdown runbook for pasting into coding agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			svc := &contextx.Service{
				Store:  a.st,
				Recall: a.recall(),
				WF:     a.wf(),
			}
			out, err := svc.Build(ctx, strings.Join(args, " "), cwd(), 5)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
}

func newUseCmd() *cobra.Command {
	var run bool
	var copyFlag bool
	cmd := &cobra.Command{
		Use:   "use <workflow-id-or-name>",
		Short: "Start step-through use of a workflow (preview + insert)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()

			w, err := resolveWorkflow(ctx, a, args[0])
			if err != nil {
				return err
			}
			proj := project.Resolve(cwd())
			opt := adapt.Options{
				OldProject: w.ProjectRoot,
				NewProject: proj,
				CWD:        cwd(),
				Aliases:    a.cfg.Aliases,
			}
			fmt.Printf("workflow: %s (%d steps) origin=%s\n", displayName(w.Name, w.ID), len(w.Steps), w.Origin)
			adapted := make([]string, 0, len(w.Steps))
			for i, st := range w.Steps {
				r := adapt.Apply(st.Command, opt)
				adapted = append(adapted, r.Adapted)
				mark := " "
				if r.Changed {
					mark = "~"
				}
				warn := ""
				if adapt.IsDestructive(r.Adapted) {
					warn = "  [destructive]"
				}
				fmt.Printf("%s %2d. %s%s\n", mark, i+1, r.Adapted, warn)
				if r.Changed {
					fmt.Printf("      was: %s\n", r.Original)
				}
			}
			if run {
				if err := runexec.RunSteps(ctx, adapted, true); err != nil {
					return err
				}
				_, _ = a.st.DB().ExecContext(ctx, `
					UPDATE workflows SET use_count = use_count + 1, last_used_at = ?, success_count = success_count + 1
					WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), w.ID)
				fmt.Println("run complete")
				return nil
			}
			_, _ = a.runs().Start(ctx, w.ID)
			_, _ = a.st.DB().ExecContext(ctx, `
				UPDATE workflows SET use_count = use_count + 1, last_used_at = ? WHERE id = ?`,
				time.Now().UTC().Format(time.RFC3339Nano), w.ID)
			if len(adapted) > 0 {
				if copyFlag {
					clipboard.CopyOrPrint(adapted[0])
				} else {
					fmt.Println("--- step 1 (paste/run, then orma next) ---")
					fmt.Println(adapted[0])
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&run, "run", false, "execute all steps after typing yes (stops on failure)")
	cmd.Flags().BoolVar(&copyFlag, "copy", false, "copy first step to clipboard")
	return cmd
}

func newNextCmd() *cobra.Command {
	var skip bool
	var abort bool
	var retry bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Continue step-through workflow (retry/skip/abort supported)",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			rs := a.runs()
			if abort {
				return rs.Abort(ctx)
			}
			if retry {
				cur, err := rs.Retry(ctx)
				if err != nil {
					return err
				}
				return printStep(ctx, a, cur.WorkflowID, cur.StepIdx)
			}
			cur, err := rs.Current(ctx)
			if err != nil {
				return err
			}
			if cur == nil {
				// maybe failed
				if skip {
					return fmt.Errorf("no active run")
				}
				return fmt.Errorf("no active run (use orma use <id> first)")
			}
			if cur.Status == "failed" {
				fmt.Printf("step %d failed (exit %v). orma next --retry | --skip | --abort\n", cur.StepIdx+1, cur.LastExit)
				return fmt.Errorf("run failed")
			}
			if skip {
				cur, err = rs.Skip(ctx)
				if err != nil {
					return err
				}
			}
			w, err := a.wf().Get(ctx, cur.WorkflowID)
			if err != nil {
				return err
			}
			if cur.StepIdx >= len(w.Steps) {
				_ = rs.Complete(ctx)
				fmt.Println("workflow complete")
				return nil
			}
			return printStep(ctx, a, cur.WorkflowID, cur.StepIdx)
		},
	}
	cmd.Flags().BoolVar(&skip, "skip", false, "skip current step")
	cmd.Flags().BoolVar(&abort, "abort", false, "abort active run")
	cmd.Flags().BoolVar(&retry, "retry", false, "retry failed step")
	return cmd
}

func printStep(ctx context.Context, a *app, workflowID string, idx int) error {
	w, err := a.wf().Get(ctx, workflowID)
	if err != nil {
		return err
	}
	if idx >= len(w.Steps) {
		_ = a.runs().Complete(ctx)
		fmt.Println("workflow complete")
		return nil
	}
	proj := project.Resolve(cwd())
	r := adapt.Apply(w.Steps[idx].Command, adapt.Options{
		OldProject: w.ProjectRoot,
		NewProject: proj,
		CWD:        cwd(),
		Aliases:    a.cfg.Aliases,
	})
	if adapt.IsDestructive(r.Adapted) {
		fmt.Println("warning: destructive command")
	}
	fmt.Printf("--- step %d/%d ---\n", idx+1, len(w.Steps))
	fmt.Println(r.Adapted)
	return nil
}

func newDistillCmd() *cobra.Command {
	var last bool
	var name string
	cmd := &cobra.Command{
		Use:   "distill [session-id]",
		Short: "Distill a session into a workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			var w interface{ GetID() string }
			_ = w
			var wf *workflow.Workflow
			if last || len(args) == 0 {
				wf, err = a.distill().DistillLast(ctx, name)
			} else {
				wf, err = a.distill().DistillSession(ctx, args[0], name)
			}
			if err != nil {
				return err
			}
			_, _ = embed.ProcessQueue(ctx, a.st.DB(), a.embedder(), 50)
			fmt.Printf("distilled %s steps=%d id=%s\n", displayName(wf.Name, wf.ID), len(wf.Steps), wf.ID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&last, "last", false, "distill most recent session")
	cmd.Flags().StringVar(&name, "name", "", "workflow name")
	return cmd
}

func newDaemonCmd() *cobra.Command {
	root := &cobra.Command{Use: "daemon", Short: "Background watch + embed worker"}
	root.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start daemon in background",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if running, pid, _ := daemon.Status(cfg.DataDir); running {
				fmt.Printf("already running pid=%d\n", pid)
				return nil
			}
			return startDaemonBackground(cfg)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run daemon in foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return daemon.Run(context.Background(), cfg)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			running, pid, err := daemon.Status(cfg.DataDir)
			if err != nil {
				return err
			}
			if !running {
				return fmt.Errorf("daemon not running")
			}
			fmt.Printf("running pid=%d\n", pid)
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			running, pid, err := daemon.Status(cfg.DataDir)
			if err != nil {
				return err
			}
			if !running {
				fmt.Println("not running")
				return nil
			}
			proc, err := os.FindProcess(pid)
			if err != nil {
				return err
			}
			if err := proc.Signal(os.Interrupt); err != nil {
				_ = proc.Kill()
			}
			daemon.RemovePID(cfg.DataDir)
			fmt.Println("stopped")
			return nil
		},
	})
	return root
}

func startDaemonBackground(cfg *config.Config) error {
	bin := exePath()
	// detach from tty; inherit env so XDG paths stay consistent
	attr := &os.ProcAttr{
		Dir:   cfg.DataDir,
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, nil, nil},
	}
	p, err := os.StartProcess(bin, []string{bin, "daemon", "run"}, attr)
	if err != nil {
		return fmt.Errorf("start daemon: %w (binary %s)", err, bin)
	}
	_ = p.Release()
	// give pid file a moment
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _, _ := daemon.Status(cfg.DataDir); ok {
			fmt.Println("daemon running")
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("daemon starting")
	return nil
}

func newConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <claude-code|codex|opencode>",
		Short: "Opt-in agent adapter setup / backfill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			switch args[0] {
			case "claude-code", "claude":
				msg, err := claude.Connect(exePath())
				if err != nil {
					return err
				}
				fmt.Println(msg)
				n, err := claude.Backfill(ctx, a.st, a.cfg)
				if err != nil {
					fmt.Println("backfill:", err.Error())
				} else {
					fmt.Printf("backfill scanned %d files\n", n)
				}
				_ = startDaemonBackground(a.cfg)
			case "codex":
				n, err := codex.Backfill(ctx, a.st, a.cfg)
				if err != nil {
					fmt.Println("backfill:", err.Error())
				} else {
					fmt.Printf("backfill scanned %d session files\n", n)
				}
				_ = startDaemonBackground(a.cfg)
			case "opencode":
				n, err := opencode.Backfill(ctx, a.st, a.cfg)
				if err != nil {
					fmt.Println("backfill:", err.Error())
				} else {
					fmt.Printf("backfill scanned %d files\n", n)
				}
				_ = startDaemonBackground(a.cfg)
			default:
				return fmt.Errorf("unknown adapter %q", args[0])
			}
			return nil
		},
	}
}

func newFixCmd() *cobra.Command {
	root := &cobra.Command{Use: "fix", Short: "Browse error→fix memory"}
	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List known fixes",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			list, err := (&fix.Service{Store: a.st}).List(ctx, 50)
			if err != nil {
				return err
			}
			for _, r := range list {
				fmt.Println(fix.FormatHuman(r))
			}
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "show <id-or-fingerprint-prefix>",
		Short: "Show a fix and its resolution workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			fs := &fix.Service{Store: a.st}
			list, err := fs.List(ctx, 200)
			if err != nil {
				return err
			}
			q := args[0]
			for _, r := range list {
				if r.ID == q || strings.HasPrefix(r.ErrorFingerprint, q) || strings.Contains(r.ErrorFingerprint, q) {
					fmt.Printf("id: %s\nfp: %s\nworkflow: %s\nexamples: %d\nupdated: %s\n",
						r.ID, r.ErrorFingerprint, r.ResolutionWorkflowID, r.ExamplesCount, r.UpdatedAt)
					if r.ResolutionWorkflowID != "" {
						w, err := a.wf().Get(ctx, r.ResolutionWorkflowID)
						if err == nil {
							for i, st := range w.Steps {
								fmt.Printf("%2d. [%s] %s\n", i+1, st.Outcome, st.Command)
							}
						}
					}
					return nil
				}
			}
			return fmt.Errorf("fix not found")
		},
	})
	return root
}

func newPurgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "purge events|sessions|workflows|all",
		Short: "Delete stored memory (local)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			switch args[0] {
			case "events":
				_, err = a.st.DB().Exec(`DELETE FROM events`)
			case "sessions":
				_, err = a.st.DB().Exec(`DELETE FROM sessions`)
			case "workflows":
				_, err = a.st.DB().Exec(`DELETE FROM workflow_steps`)
				if err == nil {
					_, err = a.st.DB().Exec(`DELETE FROM workflows`)
				}
			case "all":
				for _, t := range []string{"events", "sessions", "workflow_steps", "workflows", "notes", "pins", "fixes", "embeddings", "embed_queue", "run_state"} {
					_, _ = a.st.DB().Exec(`DELETE FROM ` + t)
				}
				_, _ = a.st.DB().Exec(`DELETE FROM memory_fts`)
			default:
				return fmt.Errorf("unknown target")
			}
			return err
		},
	}
}

func newEmbedCmd() *cobra.Command {
	root := &cobra.Command{Use: "embed", Short: "Embedding model helpers"}
	root.AddCommand(&cobra.Command{
		Use:   "ensure",
		Short: "Download MiniLM ONNX + ORT from Hugging Face / GitHub if needed",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Println("downloading MiniLM ONNX + onnxruntime if missing (network)...")
			if err := embed.EnsureReady(embed.ModelsDir(cfg.DataDir)); err != nil {
				return err
			}
			e, err := embed.Open(embed.ModelsDir(cfg.DataDir))
			if err != nil {
				return err
			}
			fmt.Println("model ready:", e.Name())
			fmt.Println("onnx:", embed.ONNXReady(embed.ModelsDir(cfg.DataDir)))
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "sync",
		Short: "Process pending embed queue now",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			if err := embed.EnsureReady(embed.ModelsDir(a.cfg.DataDir)); err != nil {
				return err
			}
			ctx, cancel := withTimeout()
			defer cancel()
			pending, _ := embed.QueueStats(ctx, a.st.DB())
			n, err := embed.ProcessQueue(ctx, a.st.DB(), a.embedder(), 500)
			if err != nil {
				return err
			}
			fmt.Printf("embedded %d with %s (was pending %d)\n", n, a.embedder().Name(), pending)
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show embed queue and which model is active",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			n, err := embed.QueueStats(ctx, a.st.DB())
			if err != nil {
				return err
			}
			dir := embed.ModelsDir(a.cfg.DataDir)
			fmt.Printf("queue: %d\n", n)
			fmt.Printf("ready: %v\n", embed.ModelReady(dir))
			fmt.Printf("onnx_assets: %v\n", embed.ONNXReady(dir))
			fmt.Printf("active: %s\n", embed.ActiveModelName(dir))
			fmt.Printf("dir: %s\n", dir)
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Re-download MiniLM ONNX model (checksum verified)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := embed.ModelsDir(cfg.DataDir)
			_ = os.Remove(embed.ModelONNXPath(dir))
			fmt.Println("re-downloading MiniLM from Hugging Face...")
			if err := embed.EnsureReady(dir); err != nil {
				return err
			}
			fmt.Println("updated:", embed.ActiveModelName(dir))
			return nil
		},
	})
	return root
}

func newWorkflowCmd() *cobra.Command {
	root := &cobra.Command{Use: "workflow", Short: "Workflow helpers"}
	root.AddCommand(&cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := openApp()
			if err != nil {
				return err
			}
			defer a.close()
			ctx, cancel := withTimeout()
			defer cancel()
			return a.wf().Delete(ctx, args[0])
		},
	})
	return root
}

func resolveWorkflow(ctx context.Context, a *app, idOrName string) (*workflow.Workflow, error) {
	// try id
	if w, err := a.wf().Get(ctx, idOrName); err == nil {
		return w, nil
	}
	// try name in current project
	proj := project.Resolve(cwd())
	list, err := a.wf().ListByProject(ctx, proj, 100)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == idOrName {
			return &list[i], nil
		}
	}
	// global name scan
	rows, err := a.st.DB().QueryContext(ctx, `SELECT id FROM workflows WHERE name = ? LIMIT 1`, idOrName)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %s", idOrName)
	}
	defer rows.Close()
	if rows.Next() {
		var id string
		_ = rows.Scan(&id)
		return a.wf().Get(ctx, id)
	}
	return nil, fmt.Errorf("workflow not found: %s", idOrName)
}

func displayName(name, id string) string {
	if name != "" {
		return name
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func readStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}
