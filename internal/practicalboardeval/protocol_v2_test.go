package practicalboardeval

import (
	"net/http"
	"testing"
	"time"
)

func TestProtocolV2ProviderHTTPClient(t *testing.T) {
	transport := &RecordingTransport{}
	client := providerHTTPClient(transport)
	if client.Timeout != 5*time.Minute {
		t.Fatalf("provider timeout = %s, want 5m", client.Timeout)
	}
	if client.Transport != transport {
		t.Fatal("provider client must retain the recording/budget transport")
	}
	if client.CheckRedirect == nil || client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("provider redirects must remain disabled")
	}
	if client.Jar != nil {
		t.Fatal("provider client must not introduce cookie persistence")
	}
}
