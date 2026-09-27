package usecase

import (
	"context"
	"fmt"
	"time"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/enum"
)

type Delivery struct {
	repo   webhook.DeliveryRepository
	sender webhook.Sender
	tx     webhook.Transactor
	cfg    config.Delivery
}

func NewDelivery(repo webhook.DeliveryRepository, sender webhook.Sender, tx webhook.Transactor, cfg config.Delivery) *Delivery {
	return &Delivery{repo: repo, sender: sender, tx: tx, cfg: cfg}
}

// ProcessDue sends up to limit due deliveries one after another and returns
// how many it processed. It stops at the first delivery whose outcome could
// not be saved; that delivery stays pending and is sent again later.
func (u *Delivery) ProcessDue(ctx context.Context, limit int) (int, error) {
	due, err := u.repo.ListDue(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("list due deliveries: %w", err)
	}

	for i, d := range due {
		if err := u.deliver(ctx, d); err != nil {
			return i, fmt.Errorf("deliver %s: %w", d.ID, err)
		}
	}
	return len(due), nil
}

func (u *Delivery) deliver(ctx context.Context, d webhook.DueDelivery) error {
	result, sendErr := u.sender.Send(ctx, webhook.SendRequest{
		URL:       d.EndpointURL,
		MessageID: d.MessageID,
		EventType: d.EventType,
		Payload:   d.Payload,
	})

	attempt := webhook.CreateAttemptParam{DeliveryID: d.ID, TenantID: d.TenantID, Duration: result.Duration}
	if sendErr != nil {
		msg := sendErr.Error()
		attempt.Error = &msg
	} else {
		attempt.StatusCode = &result.StatusCode
	}
	succeeded := sendErr == nil && result.StatusCode >= 200 && result.StatusCode < 300

	return u.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := u.repo.CreateAttempt(ctx, attempt); err != nil {
			return err
		}
		return u.repo.Update(ctx, u.nextState(d, succeeded))
	})
}

func (u *Delivery) nextState(d webhook.DueDelivery, succeeded bool) webhook.UpdateDeliveryParam {
	now := time.Now()
	switch {
	case succeeded:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusSucceeded, NextAttemptAt: now}
	case d.AttemptCount+1 >= u.cfg.MaxAttempts:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusFailed, NextAttemptAt: now}
	default:
		return webhook.UpdateDeliveryParam{ID: d.ID, Status: enum.DeliveryStatusPending, NextAttemptAt: now.Add(u.cfg.RetryDelay)}
	}
}
