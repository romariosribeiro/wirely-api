package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/outbox"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func testQueue(t *testing.T, store *storage.Store) *outbox.Service {
	t.Helper()
	queue, err := outbox.New(t.TempDir(), store, &recordingSender{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(queue.Close)
	return queue
}

func TestPublicQueueTextIdempotencyStatusAndCancel(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Public queue")
	if err != nil {
		t.Fatal(err)
	}
	queue := testQueue(t, store)
	app := New(Dependencies{Store: store, Queue: queue})
	scheduledAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)
	payload := map[string]string{"recipient": "+5511999999999", "message": "Mensagem agendada", "scheduledAt": scheduledAt}
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/queue/text", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	request.Header.Set("Idempotency-Key", "customer-order-123")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.HasPrefix(response.Header().Get("Location"), "/api/queue/job_") {
		t.Fatalf("enqueue failed: %d %s", response.Code, response.Body.String())
	}
	var created storage.MessageJob
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.Status != "queued" || created.Payload.Message != "Mensagem agendada" {
		t.Fatalf("unexpected queued job: %#v err=%v", created, err)
	}

	replayRequest := httptest.NewRequest(http.MethodPost, "/api/queue/text", bytes.NewReader(body))
	replayRequest.Header.Set("Authorization", "Bearer "+instance.APIToken)
	replayRequest.Header.Set("Idempotency-Key", "customer-order-123")
	replayResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusOK || replayResponse.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("idempotent replay failed: %d %s", replayResponse.Code, replayResponse.Body.String())
	}
	var replayed storage.MessageJob
	_ = json.Unmarshal(replayResponse.Body.Bytes(), &replayed)
	if replayed.ID != created.ID {
		t.Fatalf("replay created another job: %s != %s", replayed.ID, created.ID)
	}

	conflictBody, _ := json.Marshal(map[string]string{"recipient": payload["recipient"], "message": "Outro conteúdo", "scheduledAt": scheduledAt})
	conflict := httptest.NewRequest(http.MethodPost, "/api/queue/text", bytes.NewReader(conflictBody))
	conflict.Header.Set("Authorization", "Bearer "+instance.APIToken)
	conflict.Header.Set("Idempotency-Key", "customer-order-123")
	conflictResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("expected idempotency conflict, got %d %s", conflictResponse.Code, conflictResponse.Body.String())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/queue/"+created.ID, nil)
	statusRequest.Header.Set("Authorization", "Bearer "+instance.APIToken)
	statusResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), created.ID) {
		t.Fatalf("status failed: %d %s", statusResponse.Code, statusResponse.Body.String())
	}

	otherInstance, err := store.CreateInstance(context.Background(), "Other queue")
	if err != nil {
		t.Fatal(err)
	}
	isolatedRequest := httptest.NewRequest(http.MethodGet, "/api/queue/"+created.ID, nil)
	isolatedRequest.Header.Set("Authorization", "Bearer "+otherInstance.APIToken)
	isolatedResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(isolatedResponse, isolatedRequest)
	if isolatedResponse.Code != http.StatusNotFound {
		t.Fatalf("another instance accessed job: %d %s", isolatedResponse.Code, isolatedResponse.Body.String())
	}

	cancel := httptest.NewRequest(http.MethodDelete, "/api/queue/"+created.ID, nil)
	cancel.Header.Set("Authorization", "Bearer "+instance.APIToken)
	cancelResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(cancelResponse, cancel)
	if cancelResponse.Code != http.StatusOK || !strings.Contains(cancelResponse.Body.String(), `"status":"canceled"`) {
		t.Fatalf("cancel failed: %d %s", cancelResponse.Code, cancelResponse.Body.String())
	}
}

func TestPublicQueueMediaAndAdminQueueActions(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Media queue")
	if err != nil {
		t.Fatal(err)
	}
	queue := testQueue(t, store)
	app := New(Dependencies{Store: store, Queue: queue})
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
	media := multipartRequest(t, "/api/queue/media", instance.APIToken, "+5511999999999", "photo.png", "application/octet-stream", png,
		map[string]string{"type": "image", "caption": "Imagem futura", "scheduledAt": time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)})
	media.Header.Set("Idempotency-Key", "media-request-1")
	mediaResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(mediaResponse, media)
	if mediaResponse.Code != http.StatusAccepted {
		t.Fatalf("media enqueue failed: %d %s", mediaResponse.Code, mediaResponse.Body.String())
	}
	var job storage.MessageJob
	_ = json.Unmarshal(mediaResponse.Body.Bytes(), &job)
	stored, err := store.GetMessageJob(context.Background(), instance.ID, job.ID)
	if err != nil || stored.Kind != "image" || stored.Payload.FileName != "photo.png" || stored.Payload.Caption != "Imagem futura" {
		t.Fatalf("unexpected media job: %#v %v", stored, err)
	}
	if _, err := os.Stat(stored.MediaPath); err != nil {
		t.Fatalf("queued media missing: %v", err)
	}

	cookie := authenticatedCookie(t, store, app.Handler())
	list := httptest.NewRequest(http.MethodGet, "/api/v1/instances/"+instance.ID+"/queue?status=queued", nil)
	list.AddCookie(cookie)
	listResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), job.ID) {
		t.Fatalf("admin queue list failed: %d %s", listResponse.Code, listResponse.Body.String())
	}

	cancel := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/"+instance.ID+"/queue/"+job.ID, nil)
	cancel.AddCookie(cookie)
	cancelResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(cancelResponse, cancel)
	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("admin cancel failed: %d %s", cancelResponse.Code, cancelResponse.Body.String())
	}
	if _, err := os.Stat(stored.MediaPath); !os.IsNotExist(err) {
		t.Fatalf("admin cancel did not remove media: %v", err)
	}
}

func TestAdminCanRetryFailedJob(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Retry queue")
	if err != nil {
		t.Fatal(err)
	}
	queue := testQueue(t, store)
	job, _, err := store.CreateMessageJob(context.Background(), storage.MessageJobCreate{ID: "job_retry_api", InstanceID: instance.ID,
		Kind: "text", Recipient: "5511999999999", Payload: storage.JobPayload{Message: "Retry"}, Fingerprint: "retry", ScheduledAt: time.Now().UTC(), MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextMessageJob(context.Background(), time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailMessageJob(context.Background(), claimed.ID, "failed", time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store, Queue: queue})
	cookie := authenticatedCookie(t, store, app.Handler())
	retry := httptest.NewRequest(http.MethodPost, "/api/v1/instances/"+instance.ID+"/queue/"+job.ID+"/retry", nil)
	retry.AddCookie(cookie)
	retryResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(retryResponse, retry)
	if retryResponse.Code != http.StatusAccepted || !strings.Contains(retryResponse.Body.String(), `"status":"queued"`) {
		t.Fatalf("retry failed: %d %s", retryResponse.Code, retryResponse.Body.String())
	}
}

func TestQueueRoutesRequireBearerOrAdminSession(t *testing.T) {
	store := testStore(t)
	for _, target := range []string{"/api/queue/text", "/api/queue/media", "/api/queue/job_example", "/api/v1/instances/example/queue"} {
		method := http.MethodGet
		if target == "/api/queue/text" || target == "/api/queue/media" {
			method = http.MethodPost
		}
		request := httptest.NewRequest(method, target, nil)
		response := httptest.NewRecorder()
		New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s expected 401, got %d", target, response.Code)
		}
	}
}

var _ outbox.Sender = (*recordingSender)(nil)
