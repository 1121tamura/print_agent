package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"print-agent/internal/config"
)

type Client struct {
	cfg    *config.Config
	logger *slog.Logger
	http   *http.Client
}

func NewClient(cfg *config.Config, logger *slog.Logger) *Client {
	timeout := time.Duration(cfg.Backend.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		cfg:    cfg,
		logger: logger,
		http:   &http.Client{Timeout: timeout},
	}
}

// newRequest は全リクエスト共通のヘッダーを付けた http.Request を生成する。
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.Backend.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Backend.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.Backend.APIKey)
	}
	return req, nil
}

type registerRequest struct {
	MAC      string `json:"mac"`
	Hostname string `json:"hostname"`
}

type registerResponse struct {
	AgentID string `json:"agent_id"`
}

// Register は Backend に端末情報を送信して agent_id を取得する。
// 同一MACで再登録が来た場合、Backend は既存のUUIDをそのまま返す（べき等）。
func (c *Client) Register() (string, error) {
	mac, err := primaryMAC()
	if err != nil {
		return "", fmt.Errorf("get mac address: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("get hostname: %w", err)
	}

	body, err := json.Marshal(registerRequest{MAC: mac, Hostname: hostname})
	if err != nil {
		return "", fmt.Errorf("marshal register request: %w", err)
	}

	req, err := c.newRequest(context.Background(), http.MethodPost, "/agents/register", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("post /agents/register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("register failed: status %d", resp.StatusCode)
	}

	var res registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode register response: %w", err)
	}
	if res.AgentID == "" {
		return "", fmt.Errorf("register response missing agent_id")
	}

	return res.AgentID, nil
}

// Job はジョブ詳細を表す。
type Job struct {
	JobID  string `json:"job_id"`
	PDFURL string `json:"pdf_url"`
}

// GetJob は Backend からジョブ詳細を取得する。
func (c *Client) GetJob(jobID string) (*Job, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, fmt.Sprintf("/jobs/%s", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get /jobs/%s: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get job failed: status %d", resp.StatusCode)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("decode job response: %w", err)
	}
	return &job, nil
}

// GetPDF は Backend から PDF バイナリを取得する。
func (c *Client) GetPDF(jobID string) ([]byte, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, fmt.Sprintf("/jobs/%s/pdf", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get /jobs/%s/pdf: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get pdf failed: status %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read pdf body: %w", err)
	}
	return buf.Bytes(), nil
}

// AgentStatus はイベント通知で使用するステータス定数。
const (
	StatusOnline   = "online"
	StatusPrinting = "printing"
	StatusSuccess  = "success"
	StatusError    = "error"
)

type statusRequest struct {
	Status       string `json:"status"`
	JobID        string `json:"job_id,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ReportStatus はイベント発生時に Backend へ状態を通知する。
// 定期送信は行わず、起動・印刷開始・完了・失敗のタイミングで呼ぶ。
func (c *Client) ReportStatus(status, jobID, errMsg string) error {
	body, err := json.Marshal(statusRequest{
		Status:       status,
		JobID:        jobID,
		ErrorMessage: errMsg,
	})
	if err != nil {
		return fmt.Errorf("marshal status request: %w", err)
	}

	req, err := c.newRequest(context.Background(), http.MethodPost, fmt.Sprintf("/agents/%s/status", c.cfg.Agent.ID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("post /agents/%s/status: %w", c.cfg.Agent.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("report status failed: status %d", resp.StatusCode)
	}
	return nil
}

// primaryMAC は最初に見つかった物理NICのMACアドレスを返す。
func primaryMAC() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		return iface.HardwareAddr.String(), nil
	}
	return "", fmt.Errorf("no network interface found")
}
