package verification

import (
	"strings"
	"testing"
)

func TestVerificationConfig(t *testing.T) {
	if config, err := New("", ""); err != nil || config != nil {
		t.Fatal("verification should be off by default")
	}
	for _, pair := range [][2]string{{"https://example.com", ""}, {"http://example.com", strings.Repeat("00", 32)}, {"https://example.com", "bad"}} {
		if _, err := New(pair[0], pair[1]); err == nil {
			t.Fatal("invalid config accepted", pair)
		}
	}
}
