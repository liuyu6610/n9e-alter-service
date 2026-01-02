package rulesrepo

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"n9e-alter-service/internal/config"
)

type RuleSet struct {
	N9E   config.N9EConfig   `json:"n9e"`
	Pull  config.PullConfig  `json:"pull"`
	Push  config.PushConfig  `json:"push"`
	State config.StateConfig `json:"state"`

	Silences []config.SilenceRule `json:"silences"`

	Routes   []config.RouteConfig  `json:"routes"`
	Robots   []config.RobotConfig  `json:"robots"`
	Bindings []config.BindingRule  `json:"bindings"`
	DingTalk config.DingTalkConfig `json:"dingtalk"`
}

type AuditRecord struct {
	AtUnix  int64  `json:"at_unix"`
	Action  string `json:"action"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
	Message string `json:"message"`
	Actor   string `json:"actor"`
}

type Repo struct {
	root string
}

func New(dataDir string) *Repo {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		root = "data"
	}
	return &Repo{root: filepath.Join(root, "rules")}
}

func (r *Repo) CurrentPath() string {
	return filepath.Join(r.root, "current.json")
}

func (r *Repo) VersionsDir() string {
	return filepath.Join(r.root, "versions")
}

func (r *Repo) AuditsPath() string {
	return filepath.Join(r.root, "audits.jsonl")
}

func (r *Repo) EnsureDirs() error {
	if err := os.MkdirAll(r.VersionsDir(), 0o755); err != nil {
		return err
	}
	return nil
}

func (r *Repo) LoadCurrent() (RuleSet, string, bool, error) {
	p := r.CurrentPath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuleSet{}, "", false, nil
		}
		return RuleSet{}, "", false, err
	}
	var rs RuleSet
	if err := json.Unmarshal(b, &rs); err != nil {
		return RuleSet{}, "", false, err
	}
	return rs, hashBytes(b), true, nil
}

func (r *Repo) ListVersions() ([]string, error) {
	ents, err := os.ReadDir(r.VersionsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(out)
	return out, nil
}

func (r *Repo) ReadVersion(ver string) (RuleSet, string, error) {
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return RuleSet{}, "", fmt.Errorf("version is empty")
	}
	p := filepath.Join(r.VersionsDir(), ver+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		return RuleSet{}, "", err
	}
	var rs RuleSet
	if err := json.Unmarshal(b, &rs); err != nil {
		return RuleSet{}, "", err
	}
	return rs, hashBytes(b), nil
}

func (r *Repo) Publish(rs RuleSet, message string, actor string) (string, string, error) {
	if err := r.EnsureDirs(); err != nil {
		return "", "", err
	}
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return "", "", err
	}
	ver := time.Now().UTC().Format("20060102-150405.000")
	ver = strings.ReplaceAll(ver, ":", "")
	ver = strings.ReplaceAll(ver, ".", "-")
	h := hashBytes(b)

	vp := filepath.Join(r.VersionsDir(), ver+".json")
	if err := atomicWriteFile(vp, b, 0o644); err != nil {
		return "", "", err
	}
	if err := atomicWriteFile(r.CurrentPath(), b, 0o644); err != nil {
		return "", "", err
	}
	_ = r.appendAudit(AuditRecord{AtUnix: time.Now().Unix(), Action: "publish", Version: ver, Hash: h, Message: message, Actor: actor})
	return ver, h, nil
}

func (r *Repo) Rollback(toVersion string, message string, actor string) (string, string, error) {
	rs, h, err := r.ReadVersion(toVersion)
	if err != nil {
		return "", "", err
	}
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := atomicWriteFile(r.CurrentPath(), b, 0o644); err != nil {
		return "", "", err
	}
	_ = r.appendAudit(AuditRecord{AtUnix: time.Now().Unix(), Action: "rollback", Version: strings.TrimSpace(toVersion), Hash: h, Message: message, Actor: actor})
	return strings.TrimSpace(toVersion), h, nil
}

func (r *Repo) ListAudits(limit int) ([]AuditRecord, error) {
	p := r.AuditsPath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	out := make([]AuditRecord, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if ln == "" {
			continue
		}
		var ar AuditRecord
		if err := json.Unmarshal([]byte(ln), &ar); err != nil {
			continue
		}
		out = append(out, ar)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func ApplyToConfig(cfg config.Config, rs RuleSet) config.Config {
	if strings.TrimSpace(rs.N9E.BaseURL) != "" || strings.TrimSpace(rs.N9E.APIPath) != "" || strings.TrimSpace(rs.N9E.UserToken) != "" || strings.TrimSpace(rs.N9E.Authorization) != "" || rs.N9E.TimeoutSeconds != 0 {
		cfg.N9E = rs.N9E
	}
	if rs.Pull.IntervalSeconds != 0 || rs.Pull.PageLimit != 0 || rs.Pull.MaxPages != 0 || rs.Pull.MyGroups || rs.Pull.Hours != 0 || rs.Pull.Stime != 0 || rs.Pull.Etime != 0 || strings.TrimSpace(rs.Pull.Query) != "" || strings.TrimSpace(rs.Pull.Severity) != "" || strings.TrimSpace(rs.Pull.Prods) != "" || strings.TrimSpace(rs.Pull.RuleProds) != "" || strings.TrimSpace(rs.Pull.Cate) != "" || rs.Pull.Rid != 0 || strings.TrimSpace(rs.Pull.EventIDs) != "" {
		cfg.Pull = rs.Pull
	}
	if rs.Push.Enabled || strings.TrimSpace(rs.Push.Token) != "" || rs.Push.QueueSize != 0 || rs.Push.WorkerCount != 0 || rs.Push.EnqueueTimeoutMilli != 0 {
		cfg.Push = rs.Push
	}
	if strings.TrimSpace(rs.State.SnapshotFile) != "" || rs.State.SnapshotIntervalSeconds != 0 || rs.State.RetainRecoveredSeconds != 0 || rs.State.RecoverMissCount != 0 || rs.State.Redis.Enabled || strings.TrimSpace(rs.State.Redis.Addr) != "" || strings.TrimSpace(rs.State.Redis.Password) != "" || rs.State.Redis.DB != 0 || strings.TrimSpace(rs.State.Redis.KeyPrefix) != "" || rs.State.Redis.HotTTLSeconds != 0 || rs.State.Redis.TTLSeconds != 0 {
		cfg.State = rs.State
	}
	if len(rs.Silences) > 0 {
		cfg.Silences = rs.Silences
	}

	cfg.Routes = rs.Routes
	cfg.Robots = rs.Robots
	cfg.Bindings = rs.Bindings
	if strings.TrimSpace(rs.DingTalk.Webhook) != "" || strings.TrimSpace(rs.DingTalk.Secret) != "" || strings.TrimSpace(rs.DingTalk.Keyword) != "" {
		cfg.DingTalk = rs.DingTalk
	}
	return cfg
}

func ExtractFromConfig(cfg config.Config) RuleSet {
	return RuleSet{
		N9E:      cfg.N9E,
		Pull:     cfg.Pull,
		Push:     cfg.Push,
		State:    cfg.State,
		Silences: cfg.Silences,
		Routes:   cfg.Routes,
		Robots:   cfg.Robots,
		Bindings: cfg.Bindings,
		DingTalk: cfg.DingTalk,
	}
}

func (r *Repo) appendAudit(ar AuditRecord) error {
	b, err := json.Marshal(ar)
	if err != nil {
		return err
	}
	line := append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(r.AuditsPath()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.AuditsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(line)
	return err
}

func atomicWriteFile(path string, b []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func hashBytes(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}
