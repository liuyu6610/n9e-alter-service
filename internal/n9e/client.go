package n9e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"n9e-alter-service/internal/config"
)

type Client struct {
	baseURL       string
	apiPath       string
	userToken     string
	authorization string
	timeout       time.Duration
	verifyTLS     bool
	hc            *http.Client
}

type CurEvent struct {
	ID               int64          `json:"id"`
	Hash             string         `json:"hash"`
	RuleID           int64          `json:"rule_id"`
	RuleName         string         `json:"rule_name"`
	Severity         int            `json:"severity"`
	GroupID          int64          `json:"group_id"`
	GroupName        string         `json:"group_name"`
	TargetIdent      string         `json:"target_ident"`
	TargetNote       string         `json:"target_note"`
	FirstTriggerTime int64          `json:"first_trigger_time"`
	TriggerTime      int64          `json:"trigger_time"`
	Cluster          string         `json:"cluster"`
	Tags             any            `json:"tags"`
	TagsMap          map[string]any `json:"tags_map"`
}

type listResp struct {
	Err any `json:"err"`
	Dat struct {
		List  []CurEvent `json:"list"`
		Total int        `json:"total"`
	} `json:"dat"`
}

func New(cfg config.N9EConfig) *Client {
	verifyTLS := cfg.VerifyTLS
	tr := &http.Transport{}
	if !verifyTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	hc := &http.Client{Transport: tr}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		baseURL:       strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiPath:       cfg.APIPath,
		userToken:     strings.TrimSpace(cfg.UserToken),
		authorization: strings.TrimSpace(cfg.Authorization),
		timeout:       timeout,
		verifyTLS:     verifyTLS,
		hc:            hc,
	}
}

func (c *Client) FetchCurEvents(ctx context.Context, pull config.PullConfig) ([]CurEvent, int, error) {
	limit := pull.PageLimit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	maxPages := pull.MaxPages
	if maxPages <= 0 {
		maxPages = 2000
	}

	apiPath := strings.TrimSpace(c.apiPath)
	if apiPath == "" {
		apiPath = "/api/n9e/alert-cur-events/list"
	}
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}

	base := strings.TrimRight(c.baseURL, "/")
	if base == "" {
		return nil, 0, fmt.Errorf("n9e base url is empty")
	}

	all := make([]CurEvent, 0, limit)
	total := -1

	for p := 1; p <= maxPages; p++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(limit))
		params.Set("p", strconv.Itoa(p))

		if pull.MyGroups {
			params.Set("my_groups", "true")
		}

		if pull.Hours > 0 {
			params.Set("hours", strconv.Itoa(pull.Hours))
		} else {
			if pull.Stime > 0 {
				params.Set("stime", strconv.FormatInt(pull.Stime, 10))
			}
			if pull.Etime > 0 {
				params.Set("etime", strconv.FormatInt(pull.Etime, 10))
			}
		}

		if strings.TrimSpace(pull.Query) != "" {
			params.Set("query", pull.Query)
		}
		if strings.TrimSpace(pull.Severity) != "" {
			params.Set("severity", pull.Severity)
		}
		if strings.TrimSpace(pull.Prods) != "" {
			params.Set("prods", pull.Prods)
		}
		if strings.TrimSpace(pull.RuleProds) != "" {
			params.Set("rule_prods", pull.RuleProds)
		}
		if strings.TrimSpace(pull.Cate) != "" {
			params.Set("cate", pull.Cate)
		}
		if pull.Rid > 0 {
			params.Set("rid", strconv.FormatInt(pull.Rid, 10))
		}
		if strings.TrimSpace(pull.EventIDs) != "" {
			params.Set("event_ids", pull.EventIDs)
		}

		u := base + apiPath
		if qs := params.Encode(); qs != "" {
			u = u + "?" + qs
		}

		reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			return nil, 0, err
		}
		req.Header.Set("Accept", "application/json")
		if c.userToken != "" {
			req.Header.Set("X-User-Token", c.userToken)
		}
		if c.authorization != "" {
			req.Header.Set("Authorization", c.authorization)
		}

		resp, err := c.hc.Do(req)
		cancel()
		if err != nil {
			return nil, 0, err
		}
		b, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, 0, rerr
		}
		if resp.StatusCode >= 300 {
			return nil, 0, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
		}

		var lr listResp
		if err := json.Unmarshal(b, &lr); err != nil {
			return nil, 0, err
		}

		if lr.Err != nil {
			switch v := lr.Err.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return nil, 0, fmt.Errorf("n9e err: %s", v)
				}
			case bool:
				if v {
					return nil, 0, fmt.Errorf("n9e err: true")
				}
			default:
				bs, _ := json.Marshal(v)
				if strings.TrimSpace(string(bs)) != "null" && strings.TrimSpace(string(bs)) != "false" && strings.TrimSpace(string(bs)) != "0" && strings.TrimSpace(string(bs)) != "\"\"" {
					return nil, 0, fmt.Errorf("n9e err: %s", string(bs))
				}
			}
		}

		pageList := lr.Dat.List
		if total < 0 {
			total = lr.Dat.Total
		}

		if len(pageList) == 0 {
			break
		}

		all = append(all, pageList...)

		if total > 0 && len(all) >= total {
			break
		}
		if len(pageList) < limit {
			break
		}
	}

	if total < 0 {
		total = len(all)
	}
	return all, total, nil
}
