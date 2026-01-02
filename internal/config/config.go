package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr    string `json:"addr"`
	WebDir  string `json:"web_dir"`
	DataDir string `json:"data_dir"`

	APIToken string `json:"api_token"`

	N9E   N9EConfig   `json:"n9e"`
	Pull  PullConfig  `json:"pull"`
	Push  PushConfig  `json:"push"`
	State StateConfig `json:"state"`

	DingTalk DingTalkConfig `json:"dingtalk"`
	Silences []SilenceRule  `json:"silences"`
	Routes   []RouteConfig  `json:"routes"`
	Robots   []RobotConfig  `json:"robots"`
	Bindings []BindingRule  `json:"bindings"`
}

type SilenceRule struct {
	Name          string            `json:"name"`
	Enabled       bool              `json:"enabled"`
	RouteName     string            `json:"route_name"`
	Tags          map[string]string `json:"tags"`
	TagRegex      map[string]string `json:"tag_regex"`
	ExpiresAtUnix int64             `json:"expires_at_unix"`
}

type PushConfig struct {
	Enabled             bool   `json:"enabled"`
	Token               string `json:"token"`
	QueueSize           int    `json:"queue_size"`
	WorkerCount         int    `json:"worker_count"`
	EnqueueTimeoutMilli int    `json:"enqueue_timeout_milli"`
}

type RobotConfig struct {
	ID               string   `json:"id"`
	Webhook          string   `json:"webhook"`
	Secret           string   `json:"secret"`
	Keyword          string   `json:"keyword"`
	FallbackRobotIDs []string `json:"fallback_robot_ids"`
}

type BindingRule struct {
	Name           string            `json:"name"`
	Priority       int               `json:"priority"`
	Enabled        bool              `json:"enabled"`
	RobotID        string            `json:"robot_id"`
	RobotIDs       []string          `json:"robot_ids"`
	GroupID        int64             `json:"group_id"`
	GroupNameRegex string            `json:"group_name_regex"`
	RuleID         int64             `json:"rule_id"`
	RuleNameRegex  string            `json:"rule_name_regex"`
	RouteName      string            `json:"route_name"`
	Tags           map[string]string `json:"tags"`
	TagRegex       map[string]string `json:"tag_regex"`
}

type N9EConfig struct {
	BaseURL        string `json:"base_url"`
	APIPath        string `json:"api_path"`
	UserToken      string `json:"user_token"`
	Authorization  string `json:"authorization"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	VerifyTLS      bool   `json:"verify_tls"`
}

type DingTalkConfig struct {
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
	Keyword string `json:"keyword"`
}

type PullConfig struct {
	IntervalSeconds int    `json:"interval_seconds"`
	PageLimit       int    `json:"page_limit"`
	MaxPages        int    `json:"max_pages"`
	MyGroups        bool   `json:"my_groups"`
	Hours           int    `json:"hours"`
	Stime           int64  `json:"stime"`
	Etime           int64  `json:"etime"`
	Query           string `json:"query"`
	Severity        string `json:"severity"`
	Prods           string `json:"prods"`
	RuleProds       string `json:"rule_prods"`
	Cate            string `json:"cate"`
	Rid             int64  `json:"rid"`
	EventIDs        string `json:"event_ids"`
}

type StateConfig struct {
	SnapshotFile            string      `json:"snapshot_file"`
	SnapshotIntervalSeconds int         `json:"snapshot_interval_seconds"`
	RetainRecoveredSeconds  int         `json:"retain_recovered_seconds"`
	RecoverMissCount        int         `json:"recover_miss_count"`
	Redis                   RedisConfig `json:"redis"`
}

type RedisConfig struct {
	Enabled       bool   `json:"enabled"`
	Addr          string `json:"addr"`
	Password      string `json:"password"`
	DB            int    `json:"db"`
	KeyPrefix     string `json:"key_prefix"`
	HotTTLSeconds int    `json:"hot_ttl_seconds"`
	TTLSeconds    int    `json:"ttl_seconds"`
}

type RouteConfig struct {
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Match       RouteMatchConfig  `json:"match"`
	Dedup       DedupConfig       `json:"dedup"`
	Processors  []ProcessorConfig `json:"processors"`
	Notify      NotifyConfig      `json:"notify"`
	DailyReport DailyReportConfig `json:"daily_report"`
}

type ProcessorConfig struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`

	Drop    *DropProcessorConfig    `json:"drop"`
	Relabel *RelabelProcessorConfig `json:"relabel"`
	Update  *UpdateProcessorConfig  `json:"update"`
}

type DropProcessorConfig struct {
	When string `json:"when"`
}

type RelabelProcessorConfig struct {
	Rules []RelabelRule `json:"rules"`
}

type RelabelRule struct {
	Target  string `json:"target"`
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

type UpdateProcessorConfig struct {
	Sets []UpdateSet `json:"sets"`
}

type UpdateSet struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type RouteMatchConfig struct {
	GroupNameRegex string `json:"group_name_regex"`
	RuleNameRegex  string `json:"rule_name_regex"`
	SeverityIn     []int  `json:"severity_in"`
}

type DedupConfig struct {
	Mode             string        `json:"mode"`
	IncludeGroupID   bool          `json:"include_group_id"`
	IncludeGroupName bool          `json:"include_group_name"`
	IncludeRuleID    bool          `json:"include_rule_id"`
	IncludeRuleName  bool          `json:"include_rule_name"`
	IncludeSeverity  bool          `json:"include_severity"`
	IncludeEntity    bool          `json:"include_entity"`
	NormalizePodName bool          `json:"normalize_pod_name"`
	Rewrites         []RewriteRule `json:"rewrites"`
}

type RewriteRule struct {
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

type NotifyConfig struct {
	Enabled               bool               `json:"enabled"`
	DingTalk              DingTalkConfig     `json:"dingtalk"`
	Webhook               WebhookConfig      `json:"webhook"`
	RobotID               string             `json:"robot_id"`
	ObserveSeconds        int                `json:"observe_seconds"`
	RepeatIntervalSeconds int                `json:"repeat_interval_seconds"`
	SendRecovered         bool               `json:"send_recovered"`
	Escalations           []EscalationConfig `json:"escalations"`
}

type EscalationConfig struct {
	AfterSeconds          int           `json:"after_seconds"`
	RepeatIntervalSeconds int           `json:"repeat_interval_seconds"`
	RobotIDs              []string      `json:"robot_ids"`
	Webhook               WebhookConfig `json:"webhook"`
}

type WebhookConfig struct {
	Enabled        bool              `json:"enabled"`
	URL            string            `json:"url"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Headers        map[string]string `json:"headers"`
}

type DailyReportConfig struct {
	Enabled     bool   `json:"enabled"`
	Cron        string `json:"cron"`
	TitlePrefix string `json:"title_prefix"`
	MaxLines    int    `json:"max_lines"`
	MaxChars    int    `json:"max_chars"`
	ClearMode   string `json:"clear_mode"`
}

func Default() Config {
	return Config{
		Addr:    ":8080",
		WebDir:  "web/dist",
		DataDir: "data",
		N9E: N9EConfig{
			BaseURL:        "",
			APIPath:        "/api/n9e/alert-cur-events/list",
			UserToken:      "",
			Authorization:  "",
			TimeoutSeconds: 10,
			VerifyTLS:      true,
		},
		Pull: PullConfig{
			IntervalSeconds: 30,
			PageLimit:       200,
			MaxPages:        2000,
			MyGroups:        false,
			Hours:           0,
			Stime:           0,
			Etime:           0,
			Query:           "",
			Severity:        "",
			Prods:           "",
			RuleProds:       "",
			Cate:            "",
			Rid:             0,
			EventIDs:        "",
		},
		Push: PushConfig{
			Enabled:             false,
			Token:               "",
			QueueSize:           20000,
			WorkerCount:         8,
			EnqueueTimeoutMilli: 30000,
		},
		State: StateConfig{
			SnapshotFile:            "",
			SnapshotIntervalSeconds: 30,
			RetainRecoveredSeconds:  86400,
			RecoverMissCount:        2,
			Redis: RedisConfig{
				Enabled:       false,
				Addr:          "",
				Password:      "",
				DB:            0,
				KeyPrefix:     "n9e_alter",
				HotTTLSeconds: 86400,
				TTLSeconds:    0,
			},
		},
		DingTalk: DingTalkConfig{Webhook: "", Secret: "", Keyword: ""},
		Silences: nil,
		Routes: []RouteConfig{
			{
				Name:    "default",
				Enabled: true,
				Match: RouteMatchConfig{
					GroupNameRegex: ".*",
					RuleNameRegex:  ".*",
					SeverityIn:     nil,
				},
				Dedup: DedupConfig{
					Mode:             "n9e_hash",
					IncludeGroupID:   true,
					IncludeGroupName: true,
					IncludeRuleID:    true,
					IncludeRuleName:  true,
					IncludeSeverity:  true,
					IncludeEntity:    true,
					NormalizePodName: false,
					Rewrites:         nil,
				},
				Notify: NotifyConfig{
					Enabled:               false,
					DingTalk:              DingTalkConfig{},
					Webhook:               WebhookConfig{Enabled: false, URL: "", TimeoutSeconds: 5, Headers: nil},
					RobotID:               "",
					ObserveSeconds:        0,
					RepeatIntervalSeconds: 3600,
					SendRecovered:         true,
					Escalations:           nil,
				},
				DailyReport: DailyReportConfig{
					Enabled:     false,
					Cron:        "0 18 * * *",
					TitlePrefix: "N9E 告警日报",
					MaxLines:    50,
					MaxChars:    15000,
					ClearMode:   "reset_notified",
				},
			},
		},
		Robots:   nil,
		Bindings: nil,
		APIToken: "",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	baseDir := ""
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}

		abs, err := filepath.Abs(path)
		if err == nil {
			baseDir = filepath.Dir(abs)
		}
	}

	if v := strings.TrimSpace(os.Getenv("SERVICE_ADDR")); v != "" {
		cfg.Addr = v
	}
	if v := strings.TrimSpace(os.Getenv("WEB_DIR")); v != "" {
		cfg.WebDir = v
	}
	if v := strings.TrimSpace(os.Getenv("DATA_DIR")); v != "" {
		cfg.DataDir = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_BASE_URL")); v != "" {
		cfg.N9E.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_API_PATH")); v != "" {
		cfg.N9E.APIPath = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_USER_TOKEN")); v != "" {
		cfg.N9E.UserToken = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_TOKEN")); v != "" {
		cfg.N9E.UserToken = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_AUTHORIZATION")); v != "" {
		cfg.N9E.Authorization = v
	}
	if v := strings.TrimSpace(os.Getenv("N9E_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.N9E.TimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("N9E_VERIFY_TLS")); v != "" {
		cfg.N9E.VerifyTLS = !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}

	if v := strings.TrimSpace(os.Getenv("PULL_INTERVAL_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Pull.IntervalSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PUSH_ENABLED")); v != "" {
		cfg.Push.Enabled = !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}
	if v := strings.TrimSpace(os.Getenv("PUSH_TOKEN")); v != "" {
		cfg.Push.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("API_TOKEN")); v != "" {
		cfg.APIToken = v
	}
	if v := strings.TrimSpace(os.Getenv("PUSH_QUEUE_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Push.QueueSize = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PUSH_WORKER_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Push.WorkerCount = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PUSH_ENQUEUE_TIMEOUT_MILLI")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Push.EnqueueTimeoutMilli = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("STATE_SNAPSHOT_FILE")); v != "" {
		cfg.State.SnapshotFile = v
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_ENABLED")); v != "" {
		cfg.State.Redis.Enabled = !(v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_ADDR")); v != "" {
		cfg.State.Redis.Addr = v
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_PASSWORD")); v != "" {
		cfg.State.Redis.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_DB")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.State.Redis.DB = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_KEY_PREFIX")); v != "" {
		cfg.State.Redis.KeyPrefix = v
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_HOT_TTL_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.State.Redis.HotTTLSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("STATE_REDIS_TTL_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.State.Redis.TTLSeconds = n
		}
	}

	if cfg.State.Redis.HotTTLSeconds <= 0 {
		if cfg.State.Redis.TTLSeconds > 0 {
			cfg.State.Redis.HotTTLSeconds = cfg.State.Redis.TTLSeconds
		} else {
			cfg.State.Redis.HotTTLSeconds = 86400
		}
	}

	if v := strings.TrimSpace(os.Getenv("DINGTALK_WEBHOOK")); v != "" {
		cfg.DingTalk.Webhook = v
	}
	if v := strings.TrimSpace(os.Getenv("DINGTALK_SECRET")); v != "" {
		cfg.DingTalk.Secret = v
	}
	if v := strings.TrimSpace(os.Getenv("DINGTALK_KEYWORD")); v != "" {
		cfg.DingTalk.Keyword = v
	}

	if cfg.WebDir != "" && !filepath.IsAbs(cfg.WebDir) && baseDir != "" {
		cfg.WebDir = filepath.Join(baseDir, cfg.WebDir)
	}
	if cfg.DataDir != "" && !filepath.IsAbs(cfg.DataDir) && baseDir != "" {
		cfg.DataDir = filepath.Join(baseDir, cfg.DataDir)
	}
	if cfg.State.SnapshotFile == "" {
		cfg.State.SnapshotFile = filepath.Join(cfg.DataDir, "state.json")
	}
	if cfg.State.SnapshotFile != "" && !filepath.IsAbs(cfg.State.SnapshotFile) && baseDir != "" {
		cfg.State.SnapshotFile = filepath.Join(baseDir, cfg.State.SnapshotFile)
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return cfg, fmt.Errorf("addr is empty")
	}

	return cfg, nil
}
