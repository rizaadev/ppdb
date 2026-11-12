package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config defines connection settings for S3-compatible private object storage (R2, MinIO, S3).
type Config struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	UsePathStyle   bool
}

// Presigner generates AWS Signature Version 4 presigned PUT and GET URLs without heavy SDK dependencies.
type Presigner struct {
	endpoint       *url.URL
	publicEndpoint *url.URL
	region         string
	bucket         string
	accessKey      string
	secretKey      string
	usePathStyle   bool
	now            func() time.Time
}

// NewPresigner validates S3 endpoint configuration and returns a SigV4 URL presigner.
func NewPresigner(cfg Config) (*Presigner, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" ||
		strings.TrimSpace(cfg.Bucket) == "" ||
		strings.TrimSpace(cfg.AccessKey) == "" ||
		strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, ErrStorageNotConfigured
	}

	parsed, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint %q", cfg.Endpoint)
	}

	parsedPublic := parsed
	if rawPub := strings.TrimSpace(cfg.PublicEndpoint); rawPub != "" {
		pubURL, err := url.Parse(rawPub)
		if err != nil || pubURL.Scheme == "" || pubURL.Host == "" {
			return nil, fmt.Errorf("invalid S3 public endpoint %q", cfg.PublicEndpoint)
		}
		parsedPublic = pubURL
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto"
	}

	return &Presigner{
		endpoint:       parsed,
		publicEndpoint: parsedPublic,
		region:         region,
		bucket:         strings.TrimSpace(cfg.Bucket),
		accessKey:      strings.TrimSpace(cfg.AccessKey),
		secretKey:      strings.TrimSpace(cfg.SecretKey),
		usePathStyle:   cfg.UsePathStyle,
		now:            time.Now,
	}, nil
}

// PresignPutObject issues a time-limited PUT URL bound to an exact Content-Type header.
func (p *Presigner) PresignPutObject(key, mimeType string, ttl time.Duration) (PresignedResult, error) {
	headers := map[string]string{}
	if trimmedMime := strings.TrimSpace(mimeType); trimmedMime != "" {
		headers["content-type"] = trimmedMime
	}

	signedURL, expiresAt, err := p.presign(p.publicEndpoint, http.MethodPut, key, headers, ttl)
	if err != nil {
		return PresignedResult{}, err
	}

	requiredHeaders := map[string]string{}
	if v, ok := headers["content-type"]; ok {
		requiredHeaders["Content-Type"] = v
	}

	return PresignedResult{
		URL:             signedURL,
		StorageKey:      strings.TrimLeft(key, "/"),
		RequiredHeaders: requiredHeaders,
		ExpiresAt:       expiresAt,
	}, nil
}

// PresignGetObject issues a short-lived GET URL for private document inspection or download.
func (p *Presigner) PresignGetObject(key string, ttl time.Duration) (PresignedResult, error) {
	signedURL, expiresAt, err := p.presign(p.publicEndpoint, http.MethodGet, key, nil, ttl)
	if err != nil {
		return PresignedResult{}, err
	}

	return PresignedResult{
		URL:        signedURL,
		StorageKey: strings.TrimLeft(key, "/"),
		ExpiresAt:  expiresAt,
	}, nil
}

// PutObject uploads raw bytes directly to object storage using a short-lived SigV4 PUT URL.
func (p *Presigner) PutObject(ctx context.Context, key, mimeType string, body []byte) error {
	headers := map[string]string{}
	if trimmedMime := strings.TrimSpace(mimeType); trimmedMime != "" {
		headers["content-type"] = trimmedMime
	}
	signedURL, _, err := p.presign(p.endpoint, http.MethodPut, key, headers, 5*time.Minute)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create S3 upload request: %w", err)
	}
	req.ContentLength = int64(len(body))
	if v, ok := headers["content-type"]; ok {
		req.Header.Set("Content-Type", v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload object to storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("object storage upload failed HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// GetObject downloads raw object bytes and Content-Type directly from S3-compatible storage.
func (p *Presigner) GetObject(ctx context.Context, key string) ([]byte, string, error) {
	signedURL, _, err := p.presign(p.endpoint, http.MethodGet, key, nil, 5*time.Minute)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create S3 get request: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download object from storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", ErrObjectNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("object storage get failed HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return nil, "", err
	}
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	return data, ct, nil
}

// HeadObject queries object storage via a signed HEAD request to verify real object existence and byte size.
func (p *Presigner) HeadObject(ctx context.Context, key string) (*ObjectMetadata, error) {
	signedURL, _, err := p.presign(p.endpoint, http.MethodHead, key, nil, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, signedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create S3 head request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("head object in storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("object storage head failed HTTP %d", resp.StatusCode)
	}
	size := resp.ContentLength
	if size < 0 {
		if cl := strings.TrimSpace(resp.Header.Get("Content-Length")); cl != "" {
			if parsed, convErr := strconv.ParseInt(cl, 10, 64); convErr == nil {
				size = parsed
			}
		}
	}
	return &ObjectMetadata{
		SizeBytes:   size,
		ContentType: strings.TrimSpace(resp.Header.Get("Content-Type")),
	}, nil
}

// DeleteObject removes an object from S3-compatible storage using a signed DELETE URL.
func (p *Presigner) DeleteObject(ctx context.Context, key string) error {
	signedURL, _, err := p.presign(p.endpoint, http.MethodDelete, key, nil, 2*time.Minute)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, signedURL, nil)
	if err != nil {
		return fmt.Errorf("create S3 delete request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete object from storage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("object storage delete failed HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// BuildExportObjectKey generates a tenant-scoped object key for asynchronous Dapodik/EMIS/CSV exports.
func BuildExportObjectKey(tenantID, jobID uuid.UUID, filename string) string {
	safeName := SanitizeFilename(filename)
	return fmt.Sprintf("%s/exports/%s/%s", tenantID.String(), jobID.String(), safeName)
}

func (p *Presigner) presign(target *url.URL, method, rawKey string, signedHeaderVals map[string]string, ttl time.Duration) (string, time.Time, error) {
	if p == nil {
		return "", time.Time{}, ErrStorageNotConfigured
	}
	if target == nil {
		target = p.endpoint
	}
	cleanKey := strings.TrimLeft(strings.TrimSpace(rawKey), "/")
	if cleanKey == "" || strings.Contains(cleanKey, "..") {
		return "", time.Time{}, ErrInvalidObjectKey
	}

	expirySeconds := int(ttl.Seconds())
	if expirySeconds < 1 {
		expirySeconds = 60
	}
	if expirySeconds > 604800 {
		expirySeconds = 604800
	}

	issuedAt := p.now().UTC()
	expiresAt := issuedAt.Add(time.Duration(expirySeconds) * time.Second)
	amzDate := issuedAt.Format("20060102T150405Z")
	dateStamp := issuedAt.Format("20060102")
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, p.region)

	host, canonicalURI := p.targetHostAndURI(target, cleanKey)

	headerMap := map[string]string{"host": host}
	for k, v := range signedHeaderVals {
		headerMap[strings.ToLower(k)] = strings.TrimSpace(v)
	}
	headerNames := make([]string, 0, len(headerMap))
	for k := range headerMap {
		headerNames = append(headerNames, k)
	}
	sort.Strings(headerNames)

	var canonicalHeaders strings.Builder
	for _, name := range headerNames {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headerMap[name])
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(headerNames, ";")

	queryParams := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    p.accessKey + "/" + scope,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       strconv.Itoa(expirySeconds),
		"X-Amz-SignedHeaders": signedHeaders,
	}
	canonicalQuery := encodeCanonicalQuery(queryParams)

	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")

	signingKey := deriveSigningKey(p.secretKey, dateStamp, p.region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	finalURL := fmt.Sprintf(
		"%s://%s%s?%s&X-Amz-Signature=%s",
		target.Scheme,
		host,
		canonicalURI,
		canonicalQuery,
		signature,
	)
	return finalURL, expiresAt, nil
}

func (p *Presigner) targetHostAndURI(target *url.URL, cleanKey string) (string, string) {
	basePath := strings.TrimSuffix(target.EscapedPath(), "/")
	encodedKey := encodeS3Path(cleanKey)
	if p.usePathStyle {
		return target.Host, basePath + "/" + awsEscapeSegment(p.bucket) + "/" + encodedKey
	}
	return p.bucket + "." + target.Host, basePath + "/" + encodedKey
}

// BuildApplicationDocKey creates a tenant- and application-scoped S3 object key for uploaded documents.
func BuildApplicationDocKey(tenantID, applicationID, requirementID uuid.UUID, filename string) string {
	id := uuid.Must(uuid.NewV7())
	safeName := SanitizeFilename(filename)
	return fmt.Sprintf("%s/%s/%s/%s-%s", tenantID.String(), applicationID.String(), requirementID.String(), id.String(), safeName)
}

// ValidateApplicationDocKey verifies that storageKey stays strictly scoped to the given tenant, application, and requirement.
func ValidateApplicationDocKey(storageKey string, tenantID, applicationID, requirementID uuid.UUID) bool {
	if strings.Contains(storageKey, "..") || strings.Contains(storageKey, `\`) {
		return false
	}
	expectedPrefix := fmt.Sprintf("%s/%s/%s/", tenantID.String(), applicationID.String(), requirementID.String())
	if !strings.HasPrefix(storageKey, expectedPrefix) {
		return false
	}
	rest := strings.TrimPrefix(storageKey, expectedPrefix)
	return rest != "" && !strings.Contains(rest, "/")
}

// SanitizeFilename strips unsafe path characters and limits length while retaining file extension.
func SanitizeFilename(raw string) string {
	base := path.Base(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))
	ext := strings.ToLower(path.Ext(base))
	stem := strings.TrimSuffix(base, ext)

	cleanPart := func(input string, allowDot bool) string {
		var b strings.Builder
		for _, r := range strings.ToLower(input) {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
				b.WriteRune(r)
			case allowDot && r == '.':
				b.WriteRune(r)
			case r == ' ':
				b.WriteByte('_')
			}
		}
		return b.String()
	}

	safeStem := cleanPart(stem, false)
	if safeStem == "" {
		safeStem = "document"
	}
	if len(safeStem) > 64 {
		safeStem = safeStem[:64]
	}

	safeExt := cleanPart(ext, true)
	if len(safeExt) > 12 {
		safeExt = safeExt[:12]
	}
	return safeStem + safeExt
}

func encodeCanonicalQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, awsEscapeSegment(k)+"="+awsEscapeSegment(params[k]))
	}
	return strings.Join(parts, "&")
}

func encodeS3Path(key string) string {
	segments := strings.Split(key, "/")
	for i, seg := range segments {
		segments[i] = awsEscapeSegment(seg)
	}
	return strings.Join(segments, "/")
}

func awsEscapeSegment(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
