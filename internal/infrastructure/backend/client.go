package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	url := c.cfg.Backend.BaseURL + "/agents/register"
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
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
	url := fmt.Sprintf("%s/jobs/%s", c.cfg.Backend.BaseURL, jobID)
	resp, err := c.http.Get(url)
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
	url := fmt.Sprintf("%s/jobs/%s/pdf", c.cfg.Backend.BaseURL, jobID)
	resp, err := c.http.Get(url)
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

type resultRequest struct {
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ReportResult は印刷結果を Backend に返却する。
func (c *Client) ReportResult(jobID string, success bool, errMsg string) error {
	status := "success"
	if !success {
		status = "error"
	}

	body, err := json.Marshal(resultRequest{
		AgentID:      c.cfg.Agent.ID,
		Status:       status,
		ErrorMessage: errMsg,
	})
	if err != nil {
		return fmt.Errorf("marshal result request: %w", err)
	}

	url := fmt.Sprintf("%s/jobs/%s/result", c.cfg.Backend.BaseURL, jobID)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post /jobs/%s/result: %w", jobID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report result failed: status %d", resp.StatusCode)
	}
	return nil
}

// SendHeartbeat は Backend にハートビートを送信する。
func (c *Client) SendHeartbeat() error {
	url := fmt.Sprintf("%s/agents/%s/heartbeat", c.cfg.Backend.BaseURL, c.cfg.Agent.ID)
	resp, err := c.http.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("post heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat failed: status %d", resp.StatusCode)
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
