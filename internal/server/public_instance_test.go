package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordingConnector struct {
	instanceID string
	jid        string
	err        error
}

func (connector *recordingConnector) Connect(instanceID string) error {
	connector.instanceID = instanceID
	return connector.err
}

func (connector *recordingConnector) JID(string) string { return connector.jid }

func TestPublicInstanceReturnsOnlyBearerOwner(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Public instance")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateInstance(context.Background(), "Other instance")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	request := httptest.NewRequest(http.MethodGet, "/api/instance", nil)
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), instance.ID) || strings.Contains(response.Body.String(), other.ID) {
		t.Fatalf("unexpected instance response: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), instance.APIToken) {
		t.Fatal("instance response exposed the Bearer token")
	}
}

func TestPublicInstanceOperationsRequireBearer(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Instance operations")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/instance/connect", ""},
		{http.MethodPost, "/api/instance/disconnect", ""},
		{http.MethodDelete, "/api/instance/logout", ""},
		{http.MethodPost, "/api/instance/pair", `{"phone":"5511999999999"}`},
		{http.MethodDelete, "/api/instance/proxy", ""},
		{http.MethodGet, "/api/instance/qr", ""},
		{http.MethodGet, "/api/instance/status", ""},
		{http.MethodPost, "/api/contacts/check", `{"phones":["5511999999999"]}`},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: got %d", test.method, test.path, response.Code)
		}

		authorized := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		authorized.Header.Set("Authorization", "Bearer "+instance.APIToken)
		authorizedResponse := httptest.NewRecorder()
		app.Handler().ServeHTTP(authorizedResponse, authorized)
		if authorizedResponse.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s was not registered: got %d %s", test.method, test.path, authorizedResponse.Code, authorizedResponse.Body.String())
		}
	}
}

func TestPublicConnectConfiguresWebhookSubscriptions(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Connect subscriptions")
	if err != nil {
		t.Fatal(err)
	}
	connector := &recordingConnector{jid: "5511999999999@s.whatsapp.net"}
	app := New(Dependencies{Store: store, Connector: connector})
	eventString := "MESSAGE,SEND_MESSAGE,READ_RECEIPT,PRESENCE,HISTORY_SYNC,CHAT_PRESENCE,CALL,CONNECTION,LABEL,CONTACT,GROUP,NEWSLETTER,QRCODE"
	body := `{"webhookUrl":"https://example.com/wirely","subscribe":["` + strings.ReplaceAll(eventString, ",", `","`) + `"]}`
	request := httptest.NewRequest(http.MethodPost, "/api/instance/connect", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || connector.instanceID != instance.ID || !strings.Contains(response.Body.String(), `"eventString":"`+eventString+`"`) || !strings.Contains(response.Body.String(), connector.jid) {
		t.Fatalf("unexpected connect response: %d %s", response.Code, response.Body.String())
	}
	config, err := store.GetWebhookConfig(context.Background(), instance.ID)
	if err != nil || !config.Enabled || config.URL != "https://example.com/wirely" || len(config.Events) != 9 {
		t.Fatalf("webhook was not configured: %#v err=%v", config, err)
	}
	if strings.Contains(response.Body.String(), "whsec_") {
		t.Fatal("connect response exposed the webhook signing secret")
	}
}

func TestPublicConnectRejectsUnknownSubscription(t *testing.T) {
	store := testStore(t)
	instance, _ := store.CreateInstance(context.Background(), "Invalid subscription")
	connector := &recordingConnector{}
	app := New(Dependencies{Store: store, Connector: connector})
	request := httptest.NewRequest(http.MethodPost, "/api/instance/connect", strings.NewReader(`{"webhookUrl":"https://example.com/hook","subscribe":["UNKNOWN"]}`))
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || connector.instanceID != "" {
		t.Fatalf("unknown subscription was accepted: %d %s", response.Code, response.Body.String())
	}
}
