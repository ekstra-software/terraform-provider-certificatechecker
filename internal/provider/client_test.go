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
	token        string
	domains      map[string]*Domain
	certificates map[string]*Certificate
	order        []string
	nextID       int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{token: "secret-token", domains: map[string]*Domain{}, certificates: map[string]*Certificate{}}
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

	mux.HandleFunc("GET /api/v1/domains/{id}/certificate", func(w http.ResponseWriter, r *http.Request) {
		if !f.authorized(w, r) {
			return
		}

		if domain := f.find(r.PathValue("id")); domain != nil {
			// Like the real endpoint: a domain without a successful check is a 404 as well.
			if certificate, ok := f.certificates[domain.ID]; ok {
				writeJSON(w, http.StatusOK, map[string]any{"data": certificate})

				return
			}

			writeJSON(w, http.StatusNotFound, map[string]any{"message": "This domain has no successful check yet."})

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

// check records a successful check for a domain, with the certificate it saw.
func (f *fakeAPI) check(domain *Domain, certificate *Certificate) {
	f.certificates[domain.ID] = certificate
	checkedAt := "2026-07-23T07:00:00+00:00"
	domain.LastCheckedAt = &checkedAt
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

func TestGetCertificateReadsEveryField(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("example.com")
	api.check(domain, &Certificate{
		Subject:            ptr("example.com"),
		Issuer:             ptr("R11"),
		IssuerOrganization: ptr("Let's Encrypt"),
		SerialNumber:       ptr("057705f414f6b319631a81bcb32158c0267d"),
		ValidFrom:          ptr("2026-07-23T06:20:11+00:00"),
		ValidTo:            ptr("2026-09-21T06:20:11+00:00"),
		Names:              []string{"www.example.com", "example.com"},
		FingerprintSHA256:  ptr("9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"),
		FingerprintSHA1:    ptr("a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"),
		SignatureAlgorithm: ptr("RSA-SHA256"),
		KeyType:            ptr("RSA"),
		KeyBits:            ptr(int64(2048)),
		RemoteAddress:      ptr("192.0.2.10:443"),
	})
	client := newTestClient(t, api)

	got, err := client.GetCertificate(context.Background(), domain.ID)
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	if got.Subject == nil || *got.Subject != "example.com" {
		t.Fatalf("unexpected subject: %v", got.Subject)
	}
	if got.ValidTo == nil || *got.ValidTo != "2026-09-21T06:20:11+00:00" {
		t.Fatalf("unexpected valid_to: %v", got.ValidTo)
	}
	if len(got.Names) != 2 || got.Names[0] != "www.example.com" || got.Names[1] != "example.com" {
		t.Fatalf("unexpected names: %v", got.Names)
	}
	if got.KeyBits == nil || *got.KeyBits != 2048 {
		t.Fatalf("unexpected key_bits: %v", got.KeyBits)
	}
	if got.FingerprintSHA256 == nil || *got.FingerprintSHA256 != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatalf("unexpected sha256 fingerprint: %v", got.FingerprintSHA256)
	}
}

// Before the first successful check the API has no certificate to show and answers 404 —
// the same status as for an unknown domain, which is why the data source looks the domain
// up first and only then treats a 404 as "not checked yet".
func TestGetCertificateBeforeFirstCheckIsNotFound(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("fresh.example")
	client := newTestClient(t, api)

	_, err := client.GetCertificate(context.Background(), domain.ID)
	if !IsNotFound(err) {
		t.Fatalf("expected not-found before the first check, got: %v", err)
	}

	if _, err := client.GetDomain(context.Background(), domain.ID); err != nil {
		t.Fatalf("the domain itself must still resolve: %v", err)
	}
}

// A check whose stored payload cannot be read any more answers with every field null; the
// client must pass that through as nils rather than choke on it.
func TestGetCertificateWithNullFields(t *testing.T) {
	api := newFakeAPI()
	domain := api.add("stale.example")
	api.check(domain, &Certificate{})
	client := newTestClient(t, api)

	got, err := client.GetCertificate(context.Background(), domain.ID)
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	if got.Subject != nil || got.KeyBits != nil || got.Names != nil {
		t.Fatalf("expected nil fields, got %+v", got)
	}
}

func ptr[T any](v T) *T {
	return &v
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
