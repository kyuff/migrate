package migrate

import "log/slog"

// Config holds optional settings for a Migrator.
type Config struct {
	Logger *slog.Logger
}

// Option configures a Migrator.
type Option func(*Config)

// WithLogger sets the logger used by the Migrator.
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *Config) {
		cfg.Logger = logger
	}
}

func defaultConfig() *Config {
	return applyOptions(&Config{},
		WithLogger(slog.New(slog.DiscardHandler)),
	)
}

func applyOptions(cfg *Config, opts ...Option) *Config {
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
