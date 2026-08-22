package config

import "time"

// Config is the on-disk YAML schema (plan §7).
type Config struct {
	Model      ModelConfig      `yaml:"model"`
	Channels   ChannelsConfig   `yaml:"channels"`
	Tools      ToolsConfig      `yaml:"tools"`
	Loop       LoopConfig       `yaml:"loop"`
	Heartbeat  HeartbeatConfig  `yaml:"heartbeat"`
	Middleware []string         `yaml:"middleware"`
	Skills     SkillsConfig     `yaml:"skills"`
}

type ModelConfig struct {
	Provider     string `yaml:"provider"`
	BaseURL      string `yaml:"base_url"`
	Model        string `yaml:"model"`
	APIKeySecret string `yaml:"api_key_secret"`
}

type ChannelsConfig struct {
	CLI      ChannelCLI      `yaml:"cli"`
	Telegram ChannelTelegram `yaml:"telegram"`
	Webhook  ChannelWebhook  `yaml:"webhook"`
}

type ChannelCLI struct {
	Enabled bool `yaml:"enabled"`
}

type ChannelTelegram struct {
	Enabled     bool     `yaml:"enabled"`
	TokenSecret string   `yaml:"token_secret"`
	AllowFrom   []string `yaml:"allow_from"`
}

type ChannelWebhook struct {
	Enabled     bool   `yaml:"enabled"`
	Addr        string `yaml:"addr"`
	Path        string `yaml:"path"`
	AllowPublic bool   `yaml:"allow_public"`
	// Token is loaded from secrets/webhook_token when present (not stored in YAML).
}

type ToolsConfig struct {
	Workspace WorkspaceToolConfig `yaml:"workspace"`
	HTTPFetch HTTPFetchConfig     `yaml:"http_fetch"`
	Shell     ShellConfig         `yaml:"shell"`
}

type WorkspaceToolConfig struct {
	Root string `yaml:"root"`
}

type HTTPFetchConfig struct {
	MaxBytes       int64 `yaml:"max_bytes"`
	TimeoutSeconds int   `yaml:"timeout_seconds"`
}

type ShellConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LoopConfig struct {
	MaxToolIterations     int    `yaml:"max_tool_iterations"`
	CompactTokenThreshold int    `yaml:"compact_token_threshold"`
	CompactTailMessages   int    `yaml:"compact_tail_messages"`
	Profile               string `yaml:"profile"`
	TokenCeiling          int    `yaml:"token_ceiling"`
}

type HeartbeatConfig struct {
	Enabled    bool              `yaml:"enabled"`
	Interval   string            `yaml:"interval"`
	QuietHours QuietHoursConfig  `yaml:"quiet_hours"`
	Model      string            `yaml:"model"`
}

type QuietHoursConfig struct {
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Timezone string `yaml:"timezone"`
}

type SkillsConfig struct {
	Dir string `yaml:"dir"`
}

func Default() Config {
	return Config{
		Model: ModelConfig{
			Provider:     "openai_compat",
			BaseURL:      "https://api.x.ai/v1",
			Model:        "grok-4",
			APIKeySecret: "xai",
		},
		Channels: ChannelsConfig{
			CLI:      ChannelCLI{Enabled: true},
			Telegram: ChannelTelegram{Enabled: false, TokenSecret: "telegram_bot", AllowFrom: []string{}},
			Webhook:  ChannelWebhook{Enabled: false, Addr: "127.0.0.1:8743", Path: "/hook", AllowPublic: false},
		},
		Tools: ToolsConfig{
			Workspace: WorkspaceToolConfig{Root: "workspace"},
			HTTPFetch: HTTPFetchConfig{MaxBytes: 2_000_000, TimeoutSeconds: 30},
			Shell:     ShellConfig{Enabled: false},
		},
		Loop: LoopConfig{
			MaxToolIterations:     12,
			CompactTokenThreshold: 8000,
			CompactTailMessages:   10,
			Profile:               "interactive",
			TokenCeiling:          0,
		},
		Heartbeat: HeartbeatConfig{
			Enabled:  false,
			Interval: "1h",
			QuietHours: QuietHoursConfig{
				Start: "23:00", End: "07:00", Timezone: "Local",
			},
		},
		Middleware: []string{"audit", "pii", "token_ceiling"},
		Skills:     SkillsConfig{Dir: "skills"},
	}
}

func (c Config) HeartbeatInterval() (time.Duration, error) {
	if c.Heartbeat.Interval == "" {
		return time.Hour, nil
	}
	return time.ParseDuration(c.Heartbeat.Interval)
}
