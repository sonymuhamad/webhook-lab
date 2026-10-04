package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/config"
	"github.com/sonymuhamad/webhook-lab/enum"
	"github.com/sonymuhamad/webhook-lab/mock"
	"github.com/sonymuhamad/webhook-lab/usecase"
)

var deliveryConfig = config.Delivery{MaxAttempts: 3, RetryDelay: 30 * time.Second}

func TestDeliveryProcessDueOutcomes(t *testing.T) {
	tests := []struct {
		name          string
		attemptsSoFar int
		result        webhook.SendResult
		sendErr       error
		wantStatus    enum.DeliveryStatus
		wantRetry     bool
	}{
		{"2xx succeeds", 0, webhook.SendResult{StatusCode: 204}, nil, enum.DeliveryStatusSucceeded, false},
		{"5xx is retried", 0, webhook.SendResult{StatusCode: 503}, nil, enum.DeliveryStatusPending, true},
		{"3xx is retried", 0, webhook.SendResult{StatusCode: 302}, nil, enum.DeliveryStatusPending, true},
		{"no response is retried", 1, webhook.SendResult{}, errors.New("connection refused"), enum.DeliveryStatusPending, true},
		{"last attempt fails for good", 2, webhook.SendResult{StatusCode: 500}, nil, enum.DeliveryStatusFailed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			claimer := mock.NewMockClaimer(ctrl)
			repo := mock.NewMockDeliveryRepository(ctrl)
			sender := mock.NewMockSender(ctrl)
			due := webhook.DueDelivery{ID: uuid.New(), TenantID: uuid.New(), EndpointURL: "https://example.com/hook", AttemptCount: tt.attemptsSoFar}

			claimer.EXPECT().ClaimDue(gomock.Any(), 10).Return([]webhook.DueDelivery{due}, nil)
			sender.EXPECT().
				Send(gomock.Any(), gomock.Cond(func(r webhook.SendRequest) bool { return r.DeliveryID == due.ID })).
				Return(tt.result, tt.sendErr)

			var attempt webhook.CreateAttemptParam
			repo.EXPECT().CreateAttempt(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, p webhook.CreateAttemptParam) error { attempt = p; return nil })
			var update webhook.UpdateDeliveryParam
			repo.EXPECT().Update(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, p webhook.UpdateDeliveryParam) error { update = p; return nil })

			before := time.Now()
			n, err := usecase.NewDelivery(claimer, repo, sender, passthroughTx(t), deliveryConfig, config.Worker{}).ProcessDue(context.Background(), 10)

			if err != nil || n != 1 {
				t.Fatalf("ProcessDue = %d, %v; want 1, nil", n, err)
			}
			if update.ID != due.ID || update.Status != tt.wantStatus {
				t.Errorf("update = %+v, want status %s for %s", update, tt.wantStatus, due.ID)
			}
			if retried := update.NextAttemptAt.Sub(before) >= deliveryConfig.RetryDelay; retried != tt.wantRetry {
				t.Errorf("next attempt at %s, retry scheduled = %v, want %v", update.NextAttemptAt, retried, tt.wantRetry)
			}
			if (tt.sendErr != nil) != (attempt.Error != nil) || (tt.sendErr == nil) != (attempt.StatusCode != nil) {
				t.Errorf("attempt = %+v: want Error set only without a response, StatusCode set only with one", attempt)
			}
		})
	}
}

// When the outcome of one delivery cannot be saved, the rest of the batch
// must not be sent: the unsaved one stays pending and will be retried.
func TestDeliveryProcessDueStopsWhenOutcomeCannotBeSaved(t *testing.T) {
	ctrl := gomock.NewController(t)
	claimer := mock.NewMockClaimer(ctrl)
	repo := mock.NewMockDeliveryRepository(ctrl)
	sender := mock.NewMockSender(ctrl)
	due := []webhook.DueDelivery{{ID: uuid.New()}, {ID: uuid.New()}}
	dbErr := errors.New("connection reset")

	claimer.EXPECT().ClaimDue(gomock.Any(), 10).Return(due, nil)
	sender.EXPECT().Send(gomock.Any(), gomock.Any()).Return(webhook.SendResult{StatusCode: 200}, nil).Times(1)
	repo.EXPECT().CreateAttempt(gomock.Any(), gomock.Any()).Return(dbErr)

	n, err := usecase.NewDelivery(claimer, repo, sender, passthroughTx(t), deliveryConfig, config.Worker{}).ProcessDue(context.Background(), 10)

	if !errors.Is(err, dbErr) || n != 0 {
		t.Fatalf("ProcessDue = %d, %v; want 0 and an error wrapping %v", n, err, dbErr)
	}
}

// With a cap of one loop per endpoint, a second delivery to an endpoint that
// is still being sent to is handed back instead of holding another loop.
func TestDeliveryProcessDuePostponesWhenEndpointIsBusy(t *testing.T) {
	ctrl := gomock.NewController(t)
	claimer := mock.NewMockClaimer(ctrl)
	repo := mock.NewMockDeliveryRepository(ctrl)
	sender := mock.NewMockSender(ctrl)
	endpoint := uuid.New()
	first := webhook.DueDelivery{ID: uuid.New(), EndpointID: endpoint}
	second := webhook.DueDelivery{ID: uuid.New(), EndpointID: endpoint}
	deliveries := usecase.NewDelivery(claimer, repo, sender, passthroughTx(t), deliveryConfig, config.Worker{EndpointConcurrency: 1})

	sending, release := make(chan struct{}), make(chan struct{})
	claimer.EXPECT().ClaimDue(gomock.Any(), 10).Return([]webhook.DueDelivery{first}, nil)
	claimer.EXPECT().ClaimDue(gomock.Any(), 10).Return([]webhook.DueDelivery{second}, nil)
	sender.EXPECT().
		Send(gomock.Any(), gomock.Cond(func(r webhook.SendRequest) bool { return r.DeliveryID == first.ID })).
		DoAndReturn(func(context.Context, webhook.SendRequest) (webhook.SendResult, error) {
			close(sending)
			<-release
			return webhook.SendResult{StatusCode: 204}, nil
		})
	repo.EXPECT().CreateAttempt(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
	var postponedUntil time.Time
	repo.EXPECT().Postpone(gomock.Any(), second.ID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, until time.Time) error { postponedUntil = until; return nil })

	done := make(chan error)
	go func() {
		_, err := deliveries.ProcessDue(context.Background(), 10)
		done <- err
	}()
	<-sending

	before := time.Now()
	n, err := deliveries.ProcessDue(context.Background(), 10)
	if err != nil || n != 1 {
		t.Fatalf("second ProcessDue = %d, %v; want 1, nil", n, err)
	}
	if !postponedUntil.After(before) {
		t.Errorf("postponed until %s, want a time after %s", postponedUntil, before)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first ProcessDue: %v", err)
	}
}

// The cap counts sends in progress, not sends so far: once a send finishes,
// the next delivery to the same endpoint goes out.
func TestDeliveryProcessDueFreesEndpointAfterSend(t *testing.T) {
	ctrl := gomock.NewController(t)
	claimer := mock.NewMockClaimer(ctrl)
	repo := mock.NewMockDeliveryRepository(ctrl)
	sender := mock.NewMockSender(ctrl)
	endpoint := uuid.New()
	due := []webhook.DueDelivery{{ID: uuid.New(), EndpointID: endpoint}, {ID: uuid.New(), EndpointID: endpoint}}

	claimer.EXPECT().ClaimDue(gomock.Any(), 10).Return(due, nil)
	sender.EXPECT().Send(gomock.Any(), gomock.Any()).Return(webhook.SendResult{StatusCode: 204}, nil).Times(2)
	repo.EXPECT().CreateAttempt(gomock.Any(), gomock.Any()).Return(nil).Times(2)
	repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	n, err := usecase.NewDelivery(claimer, repo, sender, passthroughTx(t), deliveryConfig, config.Worker{EndpointConcurrency: 1}).
		ProcessDue(context.Background(), 10)

	if err != nil || n != 2 {
		t.Fatalf("ProcessDue = %d, %v; want 2, nil", n, err)
	}
}
