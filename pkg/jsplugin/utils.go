package jsplugin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
)

type volcSignRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	AccessKey string            `json:"accessKey"`
	SecretKey string            `json:"secretKey"`
	Region    string            `json:"region"`
	Service   string            `json:"service"`
	Timestamp int64             `json:"timestamp"`
}

// signHostKey carries the one host a Volcengine signing call is allowed to
// cover. It rides on the invocation context because the runtime is pooled per
// plugin while the channel base URL is known only per request.
type signHostKey struct{}

// WithSignHost binds the Volcengine signing helper to the channel's base URL
// host for this invocation. A call that does not set it cannot sign at all.
func WithSignHost(ctx context.Context, baseURL string) context.Context {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return ctx
	}
	return context.WithValue(ctx, signHostKey{}, canonicalHost(parsed))
}

func signHostFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if host, ok := ctx.Value(signHostKey{}).(string); ok {
		return host
	}
	return ""
}

func injectGlobals(runtime *sobek.Runtime, identity func() string, now func() time.Time, logOutput func(string), currentSignHost func() string) error {
	utils := map[string]any{
		"hasCapability": HasCapability,
		"json": map[string]any{
			"clone": func(call sobek.FunctionCall) sobek.Value {
				if sobek.IsUndefined(call.Argument(0)) {
					panic(runtime.NewTypeError("json.clone requires a JSON value"))
				}
				budget := &jsonStateBudget{ctx: context.Background(), bytes: MaxJSONToolBytes, nodes: maxJSONToolNodes}
				value, err := newJSONStateNode(call.Argument(0), 0, budget)
				if err != nil {
					panic(runtime.NewTypeError("json.clone: %s", err))
				}
				cloned, err := value.jsValue(runtime)
				if err != nil {
					panic(runtime.NewGoError(err))
				}
				return cloned
			},
		},
		"unixNow": func() int64 { return now().Unix() },
		"jwtSignHS256": func(claims map[string]any, secret string) (string, error) {
			return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(claims)).SignedString([]byte(secret))
		},
		"hmacSHA256": func(message, secret string) string {
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte(message))
			return hex.EncodeToString(mac.Sum(nil))
		},
		"base64": func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) },
		"base64URL": func(value string) string {
			return base64.RawURLEncoding.EncodeToString([]byte(value))
		},
		"base64URLDecode": func(value string) (string, error) {
			decoded, err := base64.RawURLEncoding.DecodeString(value)
			return string(decoded), err
		},
		"uuid": func() string { return uuid.NewString() },
		"volcSignV4": func(request volcSignRequest) (map[string]string, error) {
			return signVolcV4(request, now, currentSignHost())
		},
	}
	if err := runtime.Set("utils", utils); err != nil {
		return err
	}
	console := runtime.NewObject()
	if err := console.Set("log", func(call sobek.FunctionCall) sobek.Value {
		parts := make([]string, len(call.Arguments))
		for i, argument := range call.Arguments {
			parts[i] = argument.String()
		}
		if logOutput != nil {
			logOutput(identity() + " " + strings.Join(parts, " "))
		}
		return sobek.Undefined()
	}); err != nil {
		return err
	}
	return runtime.Set("console", console)
}

func signVolcV4(request volcSignRequest, now func() time.Time, allowedHost string) (map[string]string, error) {
	parsedURL, err := url.Parse(request.URL)
	if err != nil || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid Volcengine signing URL")
	}
	// The signature covers the host, so signing an arbitrary URL hands the
	// plugin a valid credential for a host it is not allowed to call. The host
	// is pinned to the channel's own base URL, which is the only thing a plugin
	// legitimately signs; with no scope bound the helper refuses outright, so a
	// call site that forgets to declare one loses signing rather than leaking it.
	if allowedHost == "" {
		return nil, fmt.Errorf("Volcengine signing is not available for this call")
	}
	if canonicalHost(parsedURL) != allowedHost {
		return nil, fmt.Errorf("Volcengine signing URL host is not the channel host")
	}
	region := request.Region
	if region == "" {
		region = "cn-north-1"
	}
	service := request.Service
	if service == "" {
		service = "cv"
	}
	timestamp := now().UTC()
	if request.Timestamp != 0 {
		timestamp = time.Unix(request.Timestamp, 0).UTC()
	}
	xDate := timestamp.Format("20060102T150405Z")
	shortDate := timestamp.Format("20060102")
	bodyHash := sha256.Sum256([]byte(request.Body))
	requestPath := parsedURL.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}

	headers := make(map[string]string, len(request.Headers)+3)
	for name, value := range request.Headers {
		headers[strings.ToLower(name)] = strings.TrimSpace(value)
	}
	headers["host"] = parsedURL.Host
	headers["x-date"] = xDate
	headers["x-content-sha256"] = hex.EncodeToString(bodyHash[:])
	keys := make([]string, 0, len(headers))
	for name := range headers {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var canonicalHeaders strings.Builder
	for _, name := range keys {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headers[name])
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(keys, ";")
	canonicalRequest := strings.Join([]string{
		strings.ToUpper(request.Method), requestPath, parsedURL.Query().Encode(),
		canonicalHeaders.String(), signedHeaders, hex.EncodeToString(bodyHash[:]),
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	scope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, service)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%s\n%s\n%s", xDate, scope, hex.EncodeToString(canonicalHash[:]))
	sign := func(key []byte, value string) []byte {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(value))
		return mac.Sum(nil)
	}
	signingKey := sign(sign(sign([]byte(request.SecretKey), shortDate), region), service)
	signingKey = sign(signingKey, "request")
	signature := hex.EncodeToString(sign(signingKey, stringToSign))
	return map[string]string{
		"Authorization":    fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", request.AccessKey, scope, signedHeaders, signature),
		"X-Date":           xDate,
		"X-Content-Sha256": hex.EncodeToString(bodyHash[:]),
	}, nil
}
