package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vvb13a/goaudit/domain"
)

// NotifyTrigger identifies what started a run, so the notifier can honor the
// notify-on-direct / notify-on-schedule settings.
type NotifyTrigger string

const (
	NotifyDirect   NotifyTrigger = "direct"
	NotifySchedule NotifyTrigger = "schedule"
)

// Notifier delivers a summary of a finished audit to the configured channel
// (currently a Slack incoming webhook). The notification settings are per
// audit: the caller passes the audit's effective configuration, so different
// audits can target different endpoints. It is a no op when notifications are
// disabled, the trigger is not enabled, or no webhook is configured.
type Notifier struct {
	client *http.Client
}

func NewNotifier() *Notifier {
	return &Notifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Notify sends the audit summary for the given trigger using cfg. Delivery is
// best effort: the caller logs but does not fail the run on error.
func (n *Notifier) Notify(ctx context.Context, audit *domain.Audit, cfg Config, trigger NotifyTrigger) error {
	if n == nil || audit == nil {
		return nil
	}
	if !cfg.NotificationsEnabled {
		return nil
	}
	switch trigger {
	case NotifyDirect:
		if !cfg.NotifyOnDirect {
			return nil
		}
	case NotifySchedule:
		if !cfg.NotifyOnSchedule {
			return nil
		}
	}

	webhook := strings.TrimSpace(cfg.SlackWebhookURL)
	if webhook == "" {
		return nil
	}

	payload, err := json.Marshal(map[string]string{
		"text": auditNotificationText(audit),
	})
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notification webhook returned %s", resp.Status)
	}
	return nil
}

// auditNotificationText renders the Slack message body for a finished audit.
func auditNotificationText(a *domain.Audit) string {
	var issues, criticals int
	for _, u := range a.Urls {
		issues += u.Summary.TotalCount()
		criticals += u.Summary.SeverityCounts.Fatal + u.Summary.SeverityCounts.Error
	}

	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "Audit"
	}
	return fmt.Sprintf(
		"*Audit finished:* %s\nScore: %.1f\nURLs: %d\nIssues: %d (criticals: %d)\nDuration: %s",
		name,
		a.Score,
		len(a.Urls),
		issues,
		criticals,
		a.Duration.Round(time.Millisecond),
	)
}
