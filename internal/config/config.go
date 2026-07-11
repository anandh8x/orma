package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

const appName = "orma"

// Config is the on-disk TOML config.
type Config struct {
	DataDir          string            `toml:"data_dir"`
	SessionIdle      Duration          `toml:"session_idle"`
	Redact           bool              `toml:"redact"`
	BusyTimeoutMS    int               `toml:"busy_timeout_ms"`
	StderrExcerptMax int               `toml:"stderr_excerpt_max"`
	NoiseCommands    []string          `toml:"noise_commands"`
	IgnoreCWD        []string          `toml:"ignore_cwd"`
	Keybind          string            `toml:"keybind"`
	// Aliases rewrites tokens during adapt (hosts, paths, etc).
	Aliases map[string]string `toml:"aliases"`
	path    string            // config file path (not serialized)
}

// Duration wraps time.Duration for TOML strings like "20m".
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Default returns built-in defaults (paths filled for this OS).
func Default() (*Config, error) {
	dataDir, err := defaultDataDir()
	if err != nil {
		return nil, err
	}
	return &Config{
		DataDir:          dataDir,
		SessionIdle:      Duration{20 * time.Minute},
		Redact:           false,
		BusyTimeoutMS:    5000,
		StderrExcerptMax: 2048,
		NoiseCommands: []string{
			"ls", "pwd", "clear", "reset",
			"git status", "git status -sb", "git status --short",
		},
		IgnoreCWD: nil,
		Keybind:   "ctrl-g",
		Aliases:   map[string]string{},
	}, nil
}

// Path returns where the config file lives (or would live).
func (c *Config) Path() string {
	return c.path
}

// DBPath is the sqlite file under the data dir.
func (c *Config) DBPath() string {
	return filepath.Join(c.DataDir, "orma.db")
}

// Load reads config from the default location, creating defaults if missing.
func Load() (*Config, error) {
	cfgPath, err := defaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(cfgPath)
}

// LoadFrom reads or creates config at path.
func LoadFrom(cfgPath string) (*Config, error) {
	cfg, err := Default()
	if err != nil {
		return nil, err
	}
	cfg.path = cfgPath

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.path = cfgPath
	if cfg.DataDir == "" {
		cfg.DataDir, err = defaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	if cfg.SessionIdle.Duration == 0 {
		cfg.SessionIdle = Duration{20 * time.Minute}
	}
	if cfg.BusyTimeoutMS <= 0 {
		cfg.BusyTimeoutMS = 5000
	}
	if cfg.StderrExcerptMax <= 0 {
		cfg.StderrExcerptMax = 2048
	}
	if cfg.Keybind == "" {
		cfg.Keybind = "ctrl-g"
	}
	if cfg.NoiseCommands == nil {
		def, _ := Default()
		cfg.NoiseCommands = def.NoiseCommands
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	return cfg, nil
}

// Save writes the config file, creating parent dirs.
func (c *Config) Save() error {
	if c.path == "" {
		p, err := defaultConfigPath()
		if err != nil {
			return err
		}
		c.path = p
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(c)
}

// EnsureDataDir creates the data directory with user-only perms.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0o700)
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "config.toml"), nil
}

func defaultDataDir() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", appName), nil
	}
	// Linux and others: XDG data home
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", appName), nil
}
