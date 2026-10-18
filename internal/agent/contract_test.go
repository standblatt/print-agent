package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"os"
	"testing"
	"time"
)

// TestContractPairing pairs with a real server through Pair and WaitPaired, confirming the code as the dev user of
// the server's docker/dev-seed.sql, and says hello with the token it got. It runs only with STANDBLATT_CONTRACT_URL
// (scripts/contract.sh and CI set it up).
func TestContractPairing(t *testing.T) {
	url := os.Getenv("STANDBLATT_CONTRACT_URL")
	if url == "" {
		t.Skip("STANDBLATT_CONTRACT_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := &Agent{URL: url, Version: "contract", Backend: &fakeBackend{}, Client: &http.Client{Timeout: 10 * time.Second},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	p, err := a.Pair(ctx, "CONTRACT-PC")
	if err != nil {
		t.Fatal(err)
	}

	jar, _ := cookiejar.New(nil)
	user := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	call := func(method, path string, body, out any) {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequestWithContext(ctx, method, url+path, r)
		req.Header.Set("Content-Type", "application/json")
		resp, err := user.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s %s: %s %s", method, path, resp.Status, b)
		}
		if out != nil {
			json.NewDecoder(resp.Body).Decode(out)
		}
	}
	call("POST", "/api/auth/login", map[string]string{"name": "dev", "password": "devpassword"}, nil)
	var me struct {
		Tenants []struct {
			TenantID   int64  `json:"tenantId"`
			TenantName string `json:"tenantName"`
		} `json:"tenants"`
	}
	call("GET", "/api/me", nil, &me)
	if len(me.Tenants) == 0 {
		t.Fatal("the dev user has no club")
	}
	tenant := me.Tenants[0]
	var ranges []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	call("GET", fmt.Sprintf("/api/tenants/%d/ranges", tenant.TenantID), nil, &ranges)
	if len(ranges) == 0 {
		t.Fatal("the dev club has no range")
	}
	call("POST", fmt.Sprintf("/api/tenants/%d/ranges/%d/print-agents/pair", tenant.TenantID, ranges[0].ID),
		map[string]string{"code": p.Code, "name": "Contract PC"}, nil)

	paired, err := a.WaitPaired(ctx, p.Secret, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if paired.Token == "" || paired.Club != tenant.TenantName || paired.Range != ranges[0].Name {
		t.Fatalf("paired %+v, want club %q and range %q", paired, tenant.TenantName, ranges[0].Name)
	}
	a.Token = paired.Token
	info, err := a.Hello(ctx)
	if err != nil || info.Club != tenant.TenantName || info.Range != ranges[0].Name {
		t.Fatalf("hello with the paired token: %+v %v", info, err)
	}
}
