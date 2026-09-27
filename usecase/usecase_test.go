package usecase_test

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/sonymuhamad/webhook-lab/mock"
)

// passthroughTx runs the callback directly, as if the transaction always
// commits. Atomicity itself is covered by the postgres integration tests.
func passthroughTx(t *testing.T) *mock.MockTransactor {
	t.Helper()
	tx := mock.NewMockTransactor(gomock.NewController(t))
	tx.EXPECT().WithinTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }).
		AnyTimes()
	return tx
}
