package github

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/params/info"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppTokenEndpointURLs(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		wantAPI    string
		wantUpload string
	}{
		{name: "public", host: "github.com", wantAPI: "https://api.github.com", wantUpload: "https://uploads.github.com/"},
		{name: "server", host: "ghe.example.com", wantAPI: "https://ghe.example.com/api/v3", wantUpload: "https://ghe.example.com/api/uploads/"},
		{name: "cloud repository host", host: "enterprise.ghe.com", wantAPI: "https://api.enterprise.ghe.com", wantUpload: "https://uploads.enterprise.ghe.com/"},
		{name: "cloud API host", host: "api.enterprise.ghe.com", wantAPI: "https://api.enterprise.ghe.com", wantUpload: "https://uploads.enterprise.ghe.com/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PAC_GIT_PROVIDER_TOKEN_APIURL", "")
			ctx := info.StoreNS(t.Context(), "pipelines-as-code")
			run := newTestRun(t, tt.host)
			run.Info.Controller.Secret = "app-secret"
			_, err := run.Clients.Kube.CoreV1().Secrets("pipelines-as-code").Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "app-secret"},
				Data: map[string][]byte{
					"github-application-id": []byte("12345"),
					"github-private-key":    []byte(fakePrivateKey),
				},
			}, metav1.CreateOptions{})
			assert.NilError(t, err)
			originalTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			calls := 0
			http.DefaultTransport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, request.Method, http.MethodPost)
				assert.Equal(t, request.URL.String(), tt.wantAPI+"/app/installations/123/access_tokens")
				assert.Assert(t, strings.HasPrefix(request.Header.Get("Authorization"), "Bearer "))
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"token":"installation-token","expires_at":"2099-01-01T00:00:00Z"}`)),
				}, nil
			})
			provider := &Provider{Run: run}
			token, err := provider.GetAppToken(ctx, run.Clients.Kube, tt.host, 123, "pipelines-as-code")
			assert.NilError(t, err)
			assert.Equal(t, token, "installation-token")
			assert.Equal(t, calls, 1)
			assert.Equal(t, provider.Client().BaseURL(), tt.wantAPI+"/")
			assert.Equal(t, provider.Client().UploadURL(), tt.wantUpload)
		})
	}
}
