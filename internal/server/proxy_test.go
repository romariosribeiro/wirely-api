package server

import "testing"

func TestProxyValidationAndMasking(t *testing.T) {
	value, err := validateProxyURL(" socks5://wirely:secret@proxy.example:1080 ")
	if err != nil || value != "socks5://wirely:secret@proxy.example:1080" {
		t.Fatalf("valid proxy rejected: %q %v", value, err)
	}
	config := describeProxy(value)
	if !config.Configured || config.Scheme != "socks5" || config.Host != "proxy.example" || config.Port != "1080" || config.Username != "wirely" || !config.HasPassword {
		t.Fatalf("unexpected proxy description: %#v", config)
	}
	if config.URL == value || config.URL == "" {
		t.Fatal("proxy password must be masked in responses")
	}
	for _, invalid := range []string{"proxy.example:1080", "ftp://proxy.example:21", "http://proxy.example", "http://proxy.example:8080/path"} {
		if _, err := validateProxyURL(invalid); err == nil {
			t.Fatalf("invalid proxy accepted: %s", invalid)
		}
	}
}
