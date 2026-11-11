package mask

import (
	"strings"
	"testing"
)

func TestReviewDigestCredentialsAreCompletelyMasked(t *testing.T) {
	m, err := New([]string{"password", "token", "authorization"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		`Authorization: Digest username="owned-user", realm="owned-realm", nonce="owned-nonce", response="owned-response"`,
		`authorization=Digest username="owned-user", response="owned-response"`,
		"Authorization: Digest username=\"owned-user\",\r\n response=\"owned-response\"\r\nX-Request-ID: visible",
	} {
		out := m.MaskSecrets(value)
		for _, secret := range []string{"owned-user", "owned-realm", "owned-nonce", "owned-response"} {
			if strings.Contains(out, secret) {
				t.Errorf("credential remains: %s", secret)
			}
		}
		if strings.Contains(value, "visible") && !strings.Contains(out, "visible") {
			t.Error("unrelated header lost")
		}
	}
}
