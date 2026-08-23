package config

import "time"

// Config is the on-disk YAML schema (plan v4).
type Config struct {
	Model      ModelConfig      `yaml:"model"`
	Channels   ChannelsConfig   `yaml:"channels"`
	Tools      ToolsConfig      `yaml:"tools"`
	Loop       LoopConfig       `yaml:"loop"`
	Scheduler  SchedulerConfig  `yaml:"scheduler"`
	Inbox      InboxConfig      `yaml:"inbox"`
	Tasks      TasksConfig      `yaml:"tasks"`
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
}

type ToolsConfig struct {
	Workspace WorkspaceToolConfig `yaml:"workspace"`
	HTTPFetch HTTPFetchConfig     `yaml:"http_fetch"`
	Shell     ShellConfig         `yaml:"shell"`
	Tasks     ToolTasksConfig     `yaml:"tasks"`
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

type ToolTasksConfig struct {
	Enabled bool `yaml:"enabled"`
}

type LoopConfig struct {
	MaxToolIterations     int    `yaml:"max_tool_iterations"`
	CompactTokenThreshold int    `yaml:"compact_token_threshold"`
	CompactTailMessages   int    `yaml:"compact_tail_messages"`
	Profile               string `yaml:"profile"`
	TokenCeiling          int    `yaml:"token_ceiling"`
}

type SchedulerConfig struct {
	Enabled    bool             `yaml:"enabled"`
	Tick       string           `yaml:"tick"`
	QuietHours QuietHoursConfig `yaml:"quiet_hours"`
}

type QuietHoursConfig struct {
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Timezone string `yaml:"timezone"`
}

type InboxConfig struct {
	MaxPendingPerSession int    `yaml:"max_pending_per_session"`
	Drop                 string `yaml:"drop"` // oldest | refuse
}

type TasksConfig struct {
	InjectOpenLimit         int    `yaml:"inject_open_limit"`
	RunEndPolicy            string `yaml:"run_end_policy"` // block | cancel
	DefaultRequiresApproval bool   `yaml:"default_requires_approval"`
}

type SkillsConfig struct {
	Dir         string `yaml:"dir"`
	DoctorSync  bool   `yaml:"doctor_sync"`
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
			Tasks:     ToolTasksConfig{Enabled: true},
		},
		Loop: LoopConfig{
			MaxToolIterations:     12,
			CompactTokenThreshold: 8000,
			CompactTailMessages:   10,
			Profile:               "interactive",
			TokenCeiling:          0,
		},
		Scheduler: SchedulerConfig{
			Enabled: true,
			Tick:    "30s",
			QuietHours: QuietHoursConfig{
				Start: "23:00", End: "07:00", Timezone: "Local",
			},
		},
		Inbox: InboxConfig{MaxPendingPerSession: 20, Drop: "oldest"},
		Tasks: TasksConfig{InjectOpenLimit: 20, RunEndPolicy: "block", DefaultRequiresApproval: false},
		Middleware: []string{"audit", "pii", "token_ceiling"},
		Skills:     SkillsConfig{Dir: "skills", DoctorSync: true},
	}
}

func (c Config) SchedulerTick() (time.Duration, error) {
	if c.Scheduler.Tick == "" {
		return 30 * time.Second, nil
	}
	return time.ParseDuration(c.Scheduler.Tick)
}
