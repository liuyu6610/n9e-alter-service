package dingtalk

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Webhook string
	Secret  string
	Keyword string
}

type Client struct {
	hc *http.Client
}

func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{hc: &http.Client{Timeout: timeout}}
}

func (c *Client) SendMarkdown(cfg Config, title string, text string) error {
	webhook := strings.TrimSpace(cfg.Webhook)
	if webhook == "" {
		return fmt.Errorf("dingtalk webhook is empty")
	}

	keyword := strings.TrimSpace(cfg.Keyword)
	if keyword != "" && !strings.Contains(text, keyword) {
		text = keyword + "\n\n" + text
	}

	u, err := signedWebhook(webhook, strings.TrimSpace(cfg.Secret))
	if err != nil {
		return err
	}

	body := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]any{
			"title": title,
			"text":  text,
		},
	}

	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk http %d: %s", resp.StatusCode, string(rb))
	}

	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(rb, &r)
	if r.ErrCode != 0 {
		return fmt.Errorf("dingtalk errcode=%d errmsg=%s", r.ErrCode, r.ErrMsg)
	}
	return nil
}

func signedWebhook(webhook string, secret string) (string, error) {
	if secret == "" {
		return webhook, nil
	}

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	stringToSign := timestamp + "\n" + secret

	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(stringToSign))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(h.Sum(nil)))

	sep := "?"
	if strings.Contains(webhook, "?") {
		sep = "&"
	}
	return webhook + sep + "timestamp=" + timestamp + "&sign=" + sign, nil
}
