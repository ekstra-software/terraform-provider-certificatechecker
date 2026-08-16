// Copyright (c) Ekstra Software
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
fakeAPI is a tiny in-memory stand-in for the Certificate Checker API, close enough
to exercise the client: bearer auth, uuid ids, the {"data": ...} envelopes,
pagination links and Laravel-style {"message": ...} errors.
*/
type fakeAPI struct {
	token   string
	domains map[string]*Domain
	order   []string
	nextID  int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{token: "secret-token", domains: map[string]*Domain{}}
}

func (f *fakeAPI) add(hostname string) *Domain {
	f.nextID++
	id := fmt.Sprintf("0198aaaa-0000-7000-8000-%012d", f.nextID)
	enabled := true
	domain := &Domain{ID: id, Hostname: hostname, UptimeEnabled: &enabled}
	f.domains[id] = domain
	f.order = append(f.order, id)

	return domain
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/domains", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		// One domain per page, so FindDomainByHostname has to follow the next links.
		page := 1
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)

		all := make([]*Domain, 0, len(f.domains))
		for _, id := range f.order {
			if domain, ok := f.domains[id]; ok {
				all = append(all, domain)
			}
		}

		var data []*Domain
		var next *string
		if page <= len(all) {
			data = all[page-1 : page]
			if page < len(all) {
				url := fmt.Sprintf("/api/v1/domains?page=%d", page+1)
				next = &url
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"data":  data,
			"links": map[string]any{"next": next},
		})
	})

	mux.HandleFunc("GET /api/v1/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		if domain := f.find(r.PathValue("id")); domain != nil {
			writeJSON(w, http.StatusOK, map[string]any{"data": domain})

			return
		}

		writeJSON(w, http.StatusNotFound, map[string]any{"message": "No query results"})
	})

	mux.HandleFunc("POST /api/v1/domains", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		var body struct {
			Hostname      string `json:"hostname"`
			UptimeEnabled *bool  `json:"uptime_enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		domain := f.add(body.Hostname)
		// Absent means the server default (on), which add() already applied.
		if body.UptimeEnabled != nil {
			domain.UptimeEnabled = body.UptimeEnabled
		}

		writeJSON(w, http.StatusCreated, map[string]any{"data": domain})
	})

	mux.HandleFunc("PATCH /api/v1/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		domain := f.find(r.PathValue("id"))
		if domain == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "No query results"})

			return
		}

		// Like the real endpoint: the uptime flag is required, and nothing else is accepted.
		var body struct {
			Hostname      *string `json:"hostname"`
			UptimeEnabled *bool   `json:"uptime_enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.UptimeEnabled == nil || body.Hostname != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "The given data was invalid."})

			return
		}

		domain.UptimeEnabled = body.UptimeEnabled
		writeJSON(w, http.StatusOK, map[string]any{"data": domain})
	})

	mux.HandleFunc("DELETE /api/v1/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		if domain := f.find(r.PathValue("id")); domain != nil {
			delete(f.domains, domain.ID)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		writeJSON(w, http.StatusNotFound, map[string]any{"message": "No query results"})
	})

	return mux
}

func (f *fakeAPI) find(id string) *Domain {
	return f.domains[id]
}

func (f *fakeAPI) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Unauthenticated."})

		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newTestClient(t *testing.T, api *fakeAPI) *Client {
	t.Helper()

	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	return NewClient(server.URL, api.token, "test")
}

func TestCreateGetDelete(t *testing.T) {
	api := newFakeAPI()
	client := newTestClient(t, api)
	ctx := context.Background()

	created, err := client.CreateDomain(ctx, "example.com", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Hostname != "example.com" || created.ID == "" {
		t.Fatalf("unexpected created domain: %+v", created)
	}
	if created.UptimeEnabled == nil || !*created.UptimeEnabled {
		t.Fatalf("expected uptime checks on by default, got %+v", created.UptimeEnabled)
	}

	got, err := client.GetDomain(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Hostname != "example.com" {
		t.Fatalf("unexpected hostname: %q", got.Hostname)
	}

	if err := client.DeleteDomain(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := client.GetDomain(ctx, created.ID); !IsNotFound(err) {
		t.Fatalf("expected not-found after delete, got: %v", err)
	}
}

func TestUptimeChecksCanBeSetAtCreateAndChangedAfterwards(t *testing.T) {
	api := newFakeAPI()
	client := newTestClient(t, api)
	ctx := context.Background()

	off := false
	created, err := client.CreateDomain(ctx, "mail.example", &off)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.UptimeEnabled == nil || *created.UptimeEnabled {
		t.Fatalf("expected uptime checks off, got %+v", created.UptimeEnabled)
	}

	updated, err := client.UpdateDomain(ctx, created.ID, true)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.UptimeEnabled == nil || !*updated.UptimeEnabled {
		t.Fatalf("expected uptime checks on after update, got %+v", updated.UptimeEnabled)
	}

	// The change sticks server-side, rather than only showing up in the response.
	got, err := client.GetDomain(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UptimeEnabled == nil || !*got.UptimeEnabled {
		t.Fatalf("expected uptime checks on when read back, got %+v", got.UptimeEnabled)
	}
}

func TestFindDomainByHostnameFollowsPagination(t *testing.T) {
	api := newFakeAPI()
	api.add("one.example")
	api.add("two.example")
	api.add("three.example")
	client := newTestClient(t, api)

	// The fake serves one domain per page, so the last one needs page 3.
	found, err := client.FindDomainByHostname(context.Background(), "three.example")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found == nil || found.Hostname != "three.example" {
		t.Fatalf("unexpected result: %+v", found)
	}

	missing, err := client.FindDomainByHostname(context.Background(), "absent.example")
	if err != nil {
		t.Fatalf("find absent: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for absent hostname, got %+v", missing)
	}
}

func TestAPIErrorCarriesStatusAndMessage(t *testing.T) {
	api := newFakeAPI()
	client := NewClient(newTestServerURL(t, api), "wrong-token", "test")

	_, err := client.GetDomain(context.Background(), "1")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Message != "Unauthenticated." {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

// The account-level refusals (a suspended account, unaccepted terms) answer 403 with a
// stable "error" code next to the human message; the client must pass that code along.
func TestAPIErrorCarriesStableErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":        "payment_required",
			"message":      "An invoice is more than 30 days overdue, so the service is suspended.",
			"invoices_url": "https://certificate-checker.com/dashboard/settings/invoices",
		})
	}))
	t.Cleanup(server.Close)

	_, err := NewClient(server.URL, "token", "test").GetDomain(context.Background(), "1")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Code != "payment_required" || apiErr.Message == "" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

// All three flavours of 403 read differently in the diagnostic: without this, a
// suspended account looks like a plan that needs upgrading.
func TestAPIDiagnosticTellsAccountLevel403sApart(t *testing.T) {
	cases := []struct {
		code        string
		wantSummary string
	}{
		{codePaymentRequired, "Certificate Checker account is suspended"},
		{codeLegalAcceptanceRequired, "Certificate Checker terms need to be accepted"},
		{"", "Plan does not allow this API operation"},
	}

	for _, tc := range cases {
		diagnostic := apiDiagnostic("creating domain", &APIError{StatusCode: http.StatusForbidden, Code: tc.code, Message: "refused"})
		if diagnostic.Summary() != tc.wantSummary {
			t.Fatalf("code %q: expected summary %q, got %q", tc.code, tc.wantSummary, diagnostic.Summary())
		}
	}
}

func newTestServerURL(t *testing.T, api *fakeAPI) string {
	t.Helper()

	server := httptest.NewServer(api.handler())
	t.Cleanup(server.Close)

	return server.URL
}
