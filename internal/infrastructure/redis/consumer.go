package redis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"print-agent/internal/config"
)

const (
	retryBaseDelay = 2 * time.Second
	retryMaxDelay  = 30 * time.Second
)

type Consumer struct {
	cfg    *config.Config
	logger *slog.Logger
	client *goredis.Client
}

func NewConsumer(cfg *config.Config, logger *slog.Logger) *Consumer {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	return &Consumer{cfg: cfg, logger: logger, client: client}
}

// streamKey は端末専用の Stream 名を返す。
// 例: print_jobs:uuid-aaa
func (c *Consumer) streamKey() string {
	return c.cfg.Redis.StreamName + ":" + c.cfg.Agent.ID
}

// ensureGroup は Consumer Group が存在しない場合に作成する。
func (c *Consumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.streamKey(), c.cfg.Redis.ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("xgroup create: %w", err)
	}
	return nil
}

// Read は Redis Streams からジョブを1件受信して job_id を返す。
// block: 0（無限待機）でジョブが来た瞬間に即座に返す。
// シャットダウン時は ctx がキャンセルされ、Close() で接続を閉じることで即座に終了する。
// Redis 切断時はバックオフを挟んで再接続する。
func (c *Consumer) Read(ctx context.Context) (jobID string, msgID string, err error) {
	// Redis 切断時のリトライループ
	delay := retryBaseDelay
	for {
		if err := c.ensureGroup(ctx); err != nil {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			c.logger.Warn("redis ensure group failed, retrying", "err", err, "delay", delay)
			if !sleep(ctx, delay) {
				return "", "", ctx.Err()
			}
			delay = min(delay*2, retryMaxDelay)
			continue
		}
		break
	}

	streams, err := c.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    c.cfg.Redis.ConsumerGroup,
		Consumer: c.cfg.Agent.ID,
		Streams:  []string{c.streamKey(), ">"},
		Count:    1,
		Block:    0, // 無限待機: ジョブが来た瞬間に即座に返す
	}).Result()

	if err != nil {
		// Close() による強制終了 or ctx キャンセルは正常終了として扱う
		if ctx.Err() != nil || errors.Is(err, goredis.ErrClosed) {
			return "", "", ctx.Err()
		}
		return "", "", fmt.Errorf("xreadgroup: %w", err)
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return "", "", nil
	}

	msg := streams[0].Messages[0]
	id, ok := msg.Values["job_id"].(string)
	if !ok || id == "" {
		return "", "", fmt.Errorf("message missing job_id: %v", msg.Values)
	}

	return id, msg.ID, nil
}

// Ack は処理済みメッセージを ACK する。
func (c *Consumer) Ack(ctx context.Context, msgID string) error {
	if err := c.client.XAck(ctx, c.streamKey(), c.cfg.Redis.ConsumerGroup, msgID).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	return nil
}

// Close は Redis 接続を閉じる。
// ctx キャンセル時に呼ぶことで、XREADGROUP の無限待機を即座に解除する。
func (c *Consumer) Close() error {
	return c.client.Close()
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
