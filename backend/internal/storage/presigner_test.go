package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPresignerPutAndGetSigV4(t *testing.T) {
	p, err := NewPresigner(Config{
		Endpoint:     "http://localhost:9000",
		Region:       "auto",
		Bucket:       "pbdb-private-docs",
		AccessKey:    "minioadmin",
		SecretKey:    "minioadmin",
		UsePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewPresigner failed: %v", err)
	}
	fixed := time.Date(2026, 4, 10, 8, 30, 0, 0, time.UTC)
	p.now = func() time.Time { return fixed }

	putRes, err := p.PresignPutObject("tenant/app/req/kk scan.pdf", "application/pdf", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutObject failed: %v", err)
	}

	u, err := url.Parse(putRes.URL)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if u.Host != "localhost:9000" {
		t.Fatalf("unexpected host: %s", u.Host)
	}
	if !strings.HasPrefix(u.EscapedPath(), "/pbdb-private-docs/tenant/app/req/kk%20scan.pdf") {
		t.Fatalf("unexpected path: %s", u.EscapedPath())
	}
	q := u.Query()
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		t.Fatalf("unexpected algorithm: %q", q.Get("X-Amz-Algorithm"))
	}
	if q.Get("X-Amz-Credential") != "minioadmin/20260410/auto/s3/aws4_request" {
		t.Fatalf("unexpected credential: %q", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-SignedHeaders") != "content-type;host" {
		t.Fatalf("expected content-type;host signed headers, got %q", q.Get("X-Amz-SignedHeaders"))
	}
	if len(q.Get("X-Amz-Signature")) != 64 {
		t.Fatalf("expected 64-char sha256 hex signature, got %q", q.Get("X-Amz-Signature"))
	}
	if putRes.RequiredHeaders["Content-Type"] != "application/pdf" {
		t.Fatalf("expected Content-Type header, got %+v", putRes.RequiredHeaders)
	}

	getRes, err := p.PresignGetObject("tenant/app/req/kk.pdf", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetObject failed: %v", err)
	}
	getU, _ := url.Parse(getRes.URL)
	if getU.Query().Get("X-Amz-SignedHeaders") != "host" {
		t.Fatalf("expected host-only signed headers on GET, got %q", getU.Query().Get("X-Amz-SignedHeaders"))
	}
}

func TestBuildAndValidateApplicationDocAndAssetKeys(t *testing.T) {
	tenantID := uuid.New()
	appID := uuid.New()
	reqID := uuid.New()

	key := BuildApplicationDocKey(tenantID, appID, reqID, `../../evil\Kartu Keluarga (1).PDF`)
	if !ValidateApplicationDocKey(key, tenantID, appID, reqID) {
		t.Fatalf("expected valid scoped key, got %q", key)
	}
	if ValidateApplicationDocKey(key, uuid.New(), appID, reqID) {
		t.Fatal("cross-tenant key must be rejected")
	}
	if ValidateApplicationDocKey(key+"/extra", tenantID, appID, reqID) {
		t.Fatal("nested subpath key must be rejected")
	}
	if !strings.HasSuffix(key, ".pdf") {
		t.Fatalf("expected sanitized lowercase .pdf extension, got %q", key)
	}

	payID := uuid.New()
	proofKey := BuildManualProofKey(tenantID, appID, payID, "struk.png")
	if !ValidateManualProofKey(proofKey, tenantID, appID, payID) {
		t.Fatalf("expected valid proof key, got %q", proofKey)
	}
	if ValidateManualProofKey(proofKey, uuid.New(), appID, payID) {
		t.Fatal("cross-tenant proof key must fail")
	}

	logoKey := BuildTenantLogoKey(tenantID, "Logo Sekolah.webp")
	if !ValidateTenantLogoKey(logoKey, tenantID) {
		t.Fatalf("expected valid tenant logo key, got %q", logoKey)
	}
	if ValidateTenantLogoKey(logoKey, uuid.New()) {
		t.Fatal("cross-tenant logo key must fail")
	}
}

func TestPutHeadGetDeleteObjectViaPresignedSigV4(t *testing.T) {
	var mu sync.Mutex
	objects := make(map[string]struct {
		body []byte
		ct   string
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			objects[r.URL.Path] = struct {
				body []byte
				ct   string
			}{body: raw, ct: r.Header.Get("Content-Type")}
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			obj, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", obj.ct)
			w.Header().Set("Content-Length", "8")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			obj, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", obj.ct)
			_, _ = w.Write(obj.body)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	p, err := NewPresigner(Config{
		Endpoint:       srv.URL,
		PublicEndpoint: "https://storage.spmb-platform.id",
		Region:         "auto",
		Bucket:         "pbdb-private-docs",
		AccessKey:      "minioadmin",
		SecretKey:      "minioadmin",
		UsePathStyle:   true,
	})
	if err != nil {
		t.Fatalf("NewPresigner: %v", err)
	}

	key := BuildExportObjectKey(uuid.New(), uuid.New(), "dapodik.csv")
	if _, err := p.HeadObject(context.Background(), key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound before upload, got %v", err)
	}

	payload := []byte("a,b\n1,2\n")
	if err := p.PutObject(context.Background(), key, "text/csv", payload); err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	meta, err := p.HeadObject(context.Background(), key)
	if err != nil || meta.SizeBytes != int64(len(payload)) || meta.ContentType != "text/csv" {
		t.Fatalf("unexpected HeadObject result: meta=%+v err=%v", meta, err)
	}

	raw, ct, err := p.GetObject(context.Background(), key)
	if err != nil || !bytes.Equal(raw, payload) || ct != "text/csv" {
		t.Fatalf("unexpected GetObject result: ct=%q err=%v", ct, err)
	}

	if err := p.DeleteObject(context.Background(), key); err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}
	if _, err := p.HeadObject(context.Background(), key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound after delete, got %v", err)
	}

	pubGet, err := p.PresignGetObject(key, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetObject failed: %v", err)
	}
	if !strings.HasPrefix(pubGet.URL, "https://storage.spmb-platform.id/") {
		t.Fatalf("expected public endpoint host on browser presign URL, got %q", pubGet.URL)
	}
}

func TestLocalStorageSignedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ls, err := NewLocalStorage(dir, "http://localhost:8080", "local-test-secret")
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	srv := httptest.NewServer(ls)
	defer srv.Close()

	key := BuildApplicationDocKey(uuid.New(), uuid.New(), uuid.New(), "kk.pdf")
	if _, err := ls.HeadObject(context.Background(), key); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}

	putRes, err := ls.PresignPutObject(key, "application/pdf", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutObject: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPut, srv.URL+putRes.URL, strings.NewReader("%PDF-1.4 sample"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on signed PUT, got %d", resp.StatusCode)
	}

	meta, err := ls.HeadObject(context.Background(), key)
	if err != nil || meta.SizeBytes != 15 {
		t.Fatalf("expected HeadObject size 15, got %+v err=%v", meta, err)
	}

	getRes, err := ls.PresignGetObject(key, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetObject: %v", err)
	}
	getResp, err := http.Get(srv.URL + getRes.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	body, _ := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK || string(body) != "%PDF-1.4 sample" {
		t.Fatalf("unexpected GET response status=%d body=%q", getResp.StatusCode, string(body))
	}
}
