package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/webhook"
)

type fakeWebhookAPI struct {
	instanceID string
	eventID    string
	err        error
}

func (f *fakeWebhookAPI) Retry(engine.Event) error { return nil }
func (f *fakeWebhookAPI) Test(instanceID string) (string, error) {
	f.instanceID = instanceID
	return f.eventID, f.err
}

func TestPublicWebhookTestAndAuthentication(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Webhook API")
	if err != nil {
		t.Fatal(err)
	}
	tester := &fakeWebhookAPI{eventID: "evt-webhook-test"}
	app := New(Dependencies{Store: store, Webhooks: tester})
	request := httptest.NewRequest(http.MethodPost, "/api/webhook/test", nil)
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"status":"queued"`) || tester.instanceID != instance.ID {
		t.Fatalf("unexpected test response: %d %s", response.Code, response.Body.String())
	}
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/webhook/test", nil)
	unauthorizedResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorizedResponse.Code)
	}
	tester.err = webhook.ErrWebhookUnavailable
	unavailable := httptest.NewRequest(http.MethodPost, "/api/webhook/test", nil)
	unavailable.Header.Set("Authorization", "Bearer "+instance.APIToken)
	unavailableResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(unavailableResponse, unavailable)
	if unavailableResponse.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", unavailableResponse.Code, unavailableResponse.Body.String())
	}
}

func TestPublicWebhookJobAndDeliveryAreInstanceScoped(t *testing.T) {
	store := testStore(t)
	instance, _ := store.CreateInstance(context.Background(), "Webhook owner")
	other, _ := store.CreateInstance(context.Background(), "Other")
	_, err := store.EnqueueWebhookJob(context.Background(), storage.WebhookJob{
		EventID: "evt-public", InstanceID: instance.ID, Event: "message.received", PayloadJSON: `{"id":"evt-public"}`,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordWebhookDelivery(context.Background(), storage.DeliveryRecord{
		EventID: "evt-public", InstanceID: instance.ID, Event: "message.received", URL: "https://example.com/hook",
		Attempt: 1, Status: "failed", HTTPStatus: 500, Error: "HTTP 500", Duration: time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	job := httptest.NewRequest(http.MethodGet, "/api/webhook/jobs/evt-public", nil)
	job.Header.Set("Authorization", "Bearer "+instance.APIToken)
	jobResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(jobResponse, job)
	if jobResponse.Code != http.StatusOK || !strings.Contains(jobResponse.Body.String(), `"status":"queued"`) {
		t.Fatalf("unexpected job response: %d %s", jobResponse.Code, jobResponse.Body.String())
	}
	isolated := httptest.NewRequest(http.MethodGet, "/api/webhook/jobs/evt-public", nil)
	isolated.Header.Set("Authorization", "Bearer "+other.APIToken)
	isolatedResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(isolatedResponse, isolated)
	if isolatedResponse.Code != http.StatusNotFound {
		t.Fatalf("another instance accessed job: %d %s", isolatedResponse.Code, isolatedResponse.Body.String())
	}
	deliveries := httptest.NewRequest(http.MethodGet, "/api/webhook/deliveries?status=failed", nil)
	deliveries.Header.Set("Authorization", "Bearer "+instance.APIToken)
	deliveryResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(deliveryResponse, deliveries)
	if deliveryResponse.Code != http.StatusOK || !strings.Contains(deliveryResponse.Body.String(), `"total":1`) {
		t.Fatalf("unexpected delivery response: %d %s", deliveryResponse.Code, deliveryResponse.Body.String())
	}
}
