package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LocalStorage implements ObjectStorage on the local filesystem using HMAC-signed HTTP presigned URLs.
// Intended for single-VPS deployments without MinIO/S3 or local PostgreSQL development on workstations without Docker.
type LocalStorage struct {
	baseDir       string
	publicBaseURL string
	signingSecret string
	now           func() time.Time
}

// NewLocalStorage creates a verified local filesystem storage backend rooted at baseDir.
func NewLocalStorage(baseDir, publicBaseURL, signingSecret string) (*LocalStorage, error) {
	cleanDir := strings.TrimSpace(baseDir)
	if cleanDir == "" {
		cleanDir = "./var/storage"
	}
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage dir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o750); err != nil {
		return nil, fmt.Errorf("create local storage dir: %w", err)
	}
	secret := strings.TrimSpace(signingSecret)
	if secret == "" {
		return nil, fmt.Errorf("signing secret is required for local storage")
	}
	pub := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if pub == "" {
		pub = "http://localhost:8080"
	}

	return &LocalStorage{
		baseDir:       absDir,
		publicBaseURL: pub,
		signingSecret: secret,
		now:           time.Now,
	}, nil
}

func (l *LocalStorage) resolveDiskPath(rawKey string) (string, string, error) {
	if l == nil {
		return "", "", ErrStorageNotConfigured
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(rawKey), `\`, "/")
	cleanKey := strings.TrimLeft(path.Clean("/"+normalized), "/")
	if cleanKey == "" || cleanKey == "." || strings.Contains(normalized, "..") {
		return "", "", ErrInvalidObjectKey
	}
	target := filepath.Join(l.baseDir, filepath.FromSlash(cleanKey))
	rel, err := filepath.Rel(l.baseDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrInvalidObjectKey
	}
	return cleanKey, target, nil
}

func (l *LocalStorage) PresignPutObject(key, mimeType string, ttl time.Duration) (PresignedResult, error) {
	cleanKey, _, err := l.resolveDiskPath(key)
	if err != nil {
		return PresignedResult{}, err
	}
	ct := strings.TrimSpace(mimeType)
	signedURL, expiresAt := l.buildSignedURL(http.MethodPut, cleanKey, ct, ttl)
	reqHeaders := map[string]string{}
	if ct != "" {
		reqHeaders["Content-Type"] = ct
	}
	return PresignedResult{
		URL:             signedURL,
		StorageKey:      cleanKey,
		RequiredHeaders: reqHeaders,
		ExpiresAt:       expiresAt,
	}, nil
}

func (l *LocalStorage) PresignGetObject(key string, ttl time.Duration) (PresignedResult, error) {
	cleanKey, _, err := l.resolveDiskPath(key)
	if err != nil {
		return PresignedResult{}, err
	}
	signedURL, expiresAt := l.buildSignedURL(http.MethodGet, cleanKey, "", ttl)
	return PresignedResult{
		URL:        signedURL,
		StorageKey: cleanKey,
		ExpiresAt:  expiresAt,
	}, nil
}

func (l *LocalStorage) PutObject(_ context.Context, key, _ string, body []byte) error {
	_, diskPath, err := l.resolveDiskPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(diskPath, body, 0o640)
}

func (l *LocalStorage) GetObject(_ context.Context, key string) ([]byte, string, error) {
	cleanKey, diskPath, err := l.resolveDiskPath(key)
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(diskPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", err
	}
	ct := mime.TypeByExtension(filepath.Ext(cleanKey))
	if ct == "" {
		ct = http.DetectContentType(raw)
	}
	return raw, ct, nil
}

func (l *LocalStorage) HeadObject(_ context.Context, key string) (*ObjectMetadata, error) {
	cleanKey, diskPath, err := l.resolveDiskPath(key)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(diskPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrObjectNotFound
	}
	ct := mime.TypeByExtension(filepath.Ext(cleanKey))
	if ct == "" {
		f, err := os.Open(diskPath)
		if err == nil {
			buf := make([]byte, 512)
			n, _ := f.Read(buf)
			_ = f.Close()
			if n > 0 {
				ct = http.DetectContentType(buf[:n])
			}
		}
	}
	return &ObjectMetadata{
		SizeBytes:   info.Size(),
		ContentType: ct,
	}, nil
}

func (l *LocalStorage) DeleteObject(_ context.Context, key string) error {
	_, diskPath, err := l.resolveDiskPath(key)
	if err != nil {
		return err
	}
	err = os.Remove(diskPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (l *LocalStorage) buildSignedURL(method, cleanKey, contentType string, ttl time.Duration) (string, time.Time) {
	if ttl < time.Second {
		ttl = time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	expiresAt := l.now().UTC().Add(ttl)
	expStr := strconv.FormatInt(expiresAt.Unix(), 10)
	sig := l.signParams(method, cleanKey, contentType, expStr)

	q := url.Values{}
	q.Set("key", cleanKey)
	q.Set("exp", expStr)
	if contentType != "" {
		q.Set("ct", contentType)
	}
	q.Set("sig", sig)

	// Relative path works cleanly behind any tenant subdomain/custom domain or Vite dev proxy.
	return "/api/v1/storage/object?" + q.Encode(), expiresAt
}

func (l *LocalStorage) signParams(method, cleanKey, contentType, expiresUnix string) string {
	mac := hmac.New(sha256.New, []byte(l.signingSecret))
	mac.Write([]byte(strings.ToUpper(method) + "\n" + cleanKey + "\n" + contentType + "\n" + expiresUnix))
	return hex.EncodeToString(mac.Sum(nil))
}

// ServeHTTP handles signed PUT (upload) and GET (download) requests matching presigned S3 browser flows.
func (l *LocalStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cleanKey, diskPath, err := l.resolveDiskPath(q.Get("key"))
	if err != nil {
		http.Error(w, "invalid object key", http.StatusBadRequest)
		return
	}
	expStr := q.Get("exp")
	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || l.now().UTC().Unix() > expUnix {
		http.Error(w, "presigned URL expired", http.StatusForbidden)
		return
	}

	signedCT := q.Get("ct")
	expectedSig := l.signParams(r.Method, cleanKey, signedCT, expStr)
	if !hmac.Equal([]byte(expectedSig), []byte(q.Get("sig"))) {
		http.Error(w, "invalid storage signature", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodPut:
		if signedCT != "" {
			gotCT := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
			wantCT := strings.TrimSpace(strings.Split(signedCT, ";")[0])
			if !strings.EqualFold(gotCT, wantCT) {
				http.Error(w, "Content-Type mismatch", http.StatusBadRequest)
				return
			}
		}
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o750); err != nil {
			http.Error(w, "failed to prepare directory", http.StatusInternalServerError)
			return
		}
		limited := io.LimitReader(r.Body, 25<<20) // 25 MiB hard ceiling
		data, err := io.ReadAll(limited)
		if err != nil || len(data) == 0 {
			http.Error(w, "empty or unreadable body", http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(diskPath, data, 0o640); err != nil {
			http.Error(w, "failed to persist object", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodGet:
		raw, ct, err := l.GetObject(r.Context(), cleanKey)
		if err != nil {
			if errors.Is(err, ErrObjectNotFound) {
				http.Error(w, "object not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to read object", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

