package memberstatusapi_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/memberstatusapi"
)

func TestNormalizeOrigin(t *testing.T) {
	valid := map[string]string{
		"https://www.example.com":       "https://www.example.com",
		"  https://www.example.com/  ":  "https://www.example.com",
		"HTTPS://WWW.Example.COM":       "https://www.example.com",
		"https://example.com:443":       "https://example.com",
		"https://example.com:8443":      "https://example.com:8443",
		"http://localhost:3000":         "http://localhost:3000",
		"http://localhost:80":           "http://localhost",
		"http://127.0.0.1:8080":         "http://127.0.0.1:8080",
		"http://app.localhost":          "http://app.localhost",
		"https://[2001:db8::1]:8443":    "https://[2001:db8::1]:8443",
		"https://surveys.example.com/":  "https://surveys.example.com",
		"https://xn--bcher-kva.example": "https://xn--bcher-kva.example",
	}
	for in, want := range valid {
		got, err := memberstatusapi.NormalizeOrigin(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}

	invalid := []string{
		"",
		"   ",
		"example.com",
		"www.example.com:443",
		"https://",
		"https://example.com/path",
		"https://example.com/?x=1",
		"https://example.com?x=1",
		"https://example.com#frag",
		"https://*.example.com",
		"*",
		"http://example.com", // http only for loopback hosts
		"ftp://example.com",
		"https://user:pw@example.com",
		"https://exa mple.com",
		"null",
	}
	for _, in := range invalid {
		if got, err := memberstatusapi.NormalizeOrigin(in); err == nil {
			t.Errorf("%q: accepted as %q, want an error", in, got)
		}
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	text := "\n https://Surveys.Example.com/ \r\nhttps://surveys.example.com, http://localhost:3000;https://other.example\n\n"
	got, err := memberstatusapi.ParseAllowedOrigins(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"https://surveys.example.com", "http://localhost:3000", "https://other.example"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	empty, err := memberstatusapi.ParseAllowedOrigins(" \n\n ")
	if err != nil || len(empty) != 0 {
		t.Errorf("blank text: got %v, %v; want empty, nil", empty, err)
	}

	_, err = memberstatusapi.ParseAllowedOrigins("https://ok.example\nhttps://bad.example/survey\n")
	if err == nil || !strings.Contains(err.Error(), `"https://bad.example/survey"`) {
		t.Errorf("bad entry: error %v should name the entry", err)
	}

	var many []string
	for i := 0; i < 21; i++ {
		many = append(many, "https://h"+strings.Repeat("x", i)+".example")
	}
	if _, err := memberstatusapi.ParseAllowedOrigins(strings.Join(many, "\n")); err == nil {
		t.Error("21 origins: want an error")
	}
}

func TestOriginAllowed(t *testing.T) {
	allowed := []string{"https://surveys.example.com", "http://localhost:3000"}
	yes := []string{"https://surveys.example.com", "HTTPS://SURVEYS.EXAMPLE.COM", " https://surveys.example.com ", "http://localhost:3000"}
	for _, o := range yes {
		if !memberstatusapi.OriginAllowed(allowed, o) {
			t.Errorf("%q: want allowed", o)
		}
	}
	no := []string{
		"", "null",
		"https://example.com",                 // parent domain
		"https://surveys.example.com.evil.io", // suffix trick
		"https://surveys.example.com:8443",    // other port
		"http://surveys.example.com",          // other scheme
		"https://surveys.example.com/",        // browsers never send this, and it must not match
		"http://localhost",
	}
	for _, o := range no {
		if memberstatusapi.OriginAllowed(allowed, o) {
			t.Errorf("%q: want refused", o)
		}
	}
	if memberstatusapi.OriginAllowed(nil, "https://surveys.example.com") {
		t.Error("empty list must refuse everything")
	}
}
