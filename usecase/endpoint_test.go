package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	webhook "github.com/sonymuhamad/webhook-lab"
	"github.com/sonymuhamad/webhook-lab/mock"
	"github.com/sonymuhamad/webhook-lab/usecase"
)

func TestEndpointCreate(t *testing.T) {
	repo := mock.NewMockEndpointRepository(gomock.NewController(t))
	param := webhook.CreateEndpointParam{TenantID: uuid.New(), URL: "https://example.com/hook"}
	want := webhook.Endpoint{ID: uuid.New(), TenantID: param.TenantID, URL: param.URL}
	repo.EXPECT().Create(gomock.Any(), param).Return(want, nil)

	got, err := usecase.NewEndpoint(repo).Create(context.Background(), param)

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got != want {
		t.Errorf("endpoint = %+v, want %+v", got, want)
	}
}

func TestEndpointList(t *testing.T) {
	repo := mock.NewMockEndpointRepository(gomock.NewController(t))
	param := webhook.ListEndpointsParam{
		TenantID:   uuid.New(),
		Pagination: webhook.Pagination{Limit: 10, Offset: 20},
	}
	want := webhook.ListEndpointsResult{Endpoints: []webhook.Endpoint{{ID: uuid.New()}}, Total: 21}
	repo.EXPECT().ListByTenant(gomock.Any(), param).Return(want, nil)

	got, err := usecase.NewEndpoint(repo).List(context.Background(), param)

	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != want.Total || len(got.Endpoints) != 1 || got.Endpoints[0] != want.Endpoints[0] {
		t.Errorf("result = %+v, want %+v", got, want)
	}
}

func TestEndpointListWrapsRepositoryError(t *testing.T) {
	repo := mock.NewMockEndpointRepository(gomock.NewController(t))
	dbErr := errors.New("connection reset")
	repo.EXPECT().ListByTenant(gomock.Any(), gomock.Any()).Return(webhook.ListEndpointsResult{}, dbErr)

	_, err := usecase.NewEndpoint(repo).List(context.Background(), webhook.ListEndpointsParam{TenantID: uuid.New()})

	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, dbErr)
	}
}
