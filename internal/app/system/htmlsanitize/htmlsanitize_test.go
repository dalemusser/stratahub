package htmlsanitize_test

import (
	"html/template"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
)

func TestSanitize_Empty(t *testing.T) {
	result := htmlsanitize.Sanitize("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestSanitize_PlainText(t *testing.T) {
	result := htmlsanitize.Sanitize("Hello, World!")
	if result != "Hello, World!" {
		t.Errorf("expected plain text unchanged, got %q", result)
	}
}

func TestSanitize_SafeHTML(t *testing.T) {
	input := "<p><strong>Bold</strong> and <em>italic</em></p>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected safe HTML preserved, got %q", result)
	}
}

func TestSanitize_RemovesScript(t *testing.T) {
	input := "<p>Hello</p><script>alert('xss')</script>"
	result := htmlsanitize.Sanitize(input)
	if result != "<p>Hello</p>" {
		t.Errorf("expected script removed, got %q", result)
	}
}

func TestSanitize_RemovesOnclick(t *testing.T) {
	input := `<button onclick="alert('xss')">Click</button>`
	result := htmlsanitize.Sanitize(input)
	// onclick should be stripped
	if result == input {
		t.Error("expected onclick attribute to be removed")
	}
}

func TestSanitize_RemovesJavascriptHref(t *testing.T) {
	input := `<a href="javascript:alert('xss')">Click</a>`
	result := htmlsanitize.Sanitize(input)
	// javascript: href should be stripped
	if result == input {
		t.Error("expected javascript: href to be removed")
	}
}

func TestSanitize_AllowsSafeLinks(t *testing.T) {
	input := `<a href="https://example.com">Link</a>`
	result := htmlsanitize.Sanitize(input)
	// Safe link should be preserved (bluemonday adds rel="nofollow")
	if result == "" || !containsSubstring(result, "https://example.com") {
		t.Errorf("expected safe link preserved, got %q", result)
	}
}

func TestSanitize_AllowsTables(t *testing.T) {
	input := `<table><thead><tr><th>Header</th></tr></thead><tbody><tr><td>Cell</td></tr></tbody></table>`
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected table preserved, got %q", result)
	}
}

func TestSanitize_AllowsTableAttributes(t *testing.T) {
	input := `<table><tr><td colspan="2" rowspan="2">Cell</td></tr></table>`
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, `colspan="2"`) || !containsSubstring(result, `rowspan="2"`) {
		t.Errorf("expected colspan/rowspan preserved, got %q", result)
	}
}

func TestSanitize_AllowsTableClassAttribute(t *testing.T) {
	input := `<table class="my-table"><tr class="my-row"><td class="my-cell">Cell</td></tr></table>`
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, `class="my-table"`) {
		t.Errorf("expected class attribute preserved, got %q", result)
	}
}

func TestSanitize_AllowsTextFormatting(t *testing.T) {
	input := "<u>underline</u> <s>strikethrough</s> <sub>sub</sub> <sup>sup</sup> <mark>mark</mark>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected text formatting preserved, got %q", result)
	}
}

func TestSanitize_AllowsLists(t *testing.T) {
	input := "<ul><li>Item 1</li><li>Item 2</li></ul>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected list preserved, got %q", result)
	}
}

func TestSanitize_AllowsOrderedLists(t *testing.T) {
	input := "<ol><li>First</li><li>Second</li></ol>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected ordered list preserved, got %q", result)
	}
}

func TestSanitize_AllowsBlockquote(t *testing.T) {
	input := "<blockquote>A quote</blockquote>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected blockquote preserved, got %q", result)
	}
}

func TestSanitize_AllowsHeadings(t *testing.T) {
	input := "<h1>Heading 1</h1><h2>Heading 2</h2><h3>Heading 3</h3>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected headings preserved, got %q", result)
	}
}

func TestSanitize_RemovesIframe(t *testing.T) {
	input := `<p>Content</p><iframe src="https://evil.com"></iframe>`
	result := htmlsanitize.Sanitize(input)
	if containsSubstring(result, "iframe") {
		t.Error("expected iframe to be removed")
	}
	if !containsSubstring(result, "Content") {
		t.Error("expected safe content to be preserved")
	}
}

func TestSanitize_RemovesStyleTags(t *testing.T) {
	input := `<style>body { color: red; }</style><p>Text</p>`
	result := htmlsanitize.Sanitize(input)
	if containsSubstring(result, "<style>") {
		t.Error("expected style tag to be removed")
	}
}

func TestSanitize_AllowsCodeBlocks(t *testing.T) {
	input := "<pre><code>function test() {}</code></pre>"
	result := htmlsanitize.Sanitize(input)
	if result != input {
		t.Errorf("expected code blocks preserved, got %q", result)
	}
}

func TestSanitizeToHTML_ReturnsTemplateHTML(t *testing.T) {
	input := "<p>Hello</p>"
	result := htmlsanitize.SanitizeToHTML(input)
	expected := template.HTML("<p>Hello</p>")
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestSanitizeToHTML_Empty(t *testing.T) {
	result := htmlsanitize.SanitizeToHTML("")
	if result != "" {
		t.Errorf("expected empty template.HTML, got %q", result)
	}
}

func TestSanitizeToHTML_RemovesDangerousContent(t *testing.T) {
	input := "<p>Hello</p><script>alert('xss')</script>"
	result := htmlsanitize.SanitizeToHTML(input)
	if string(result) != "<p>Hello</p>" {
		t.Errorf("expected script removed, got %q", result)
	}
}

func TestIsPlainText_Empty(t *testing.T) {
	if !htmlsanitize.IsPlainText("") {
		t.Error("expected empty string to be plain text")
	}
}

func TestIsPlainText_NoTags(t *testing.T) {
	if !htmlsanitize.IsPlainText("Hello, World!") {
		t.Error("expected string without tags to be plain text")
	}
}

func TestIsPlainText_WithTags(t *testing.T) {
	if htmlsanitize.IsPlainText("<p>Hello</p>") {
		t.Error("expected string with tags to NOT be plain text")
	}
}

func TestIsPlainText_PartialTag(t *testing.T) {
	// Only has < but not >
	if !htmlsanitize.IsPlainText("5 < 10") {
		t.Error("expected string with only < to be plain text")
	}
}

func TestIsPlainText_OnlyGreaterThan(t *testing.T) {
	// Only has > but not <
	if !htmlsanitize.IsPlainText("5 > 3") {
		t.Error("expected string with only > to be plain text")
	}
}

func TestPlainTextToHTML_Empty(t *testing.T) {
	result := htmlsanitize.PlainTextToHTML("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestPlainTextToHTML_SimpleText(t *testing.T) {
	result := htmlsanitize.PlainTextToHTML("Hello, World!")
	expected := "<p>Hello, World!</p>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPlainTextToHTML_NewlinesConverted(t *testing.T) {
	result := htmlsanitize.PlainTextToHTML("Line 1\nLine 2\nLine 3")
	expected := "<p>Line 1<br>Line 2<br>Line 3</p>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPlainTextToHTML_HTMLEscaped(t *testing.T) {
	result := htmlsanitize.PlainTextToHTML("<script>alert('xss')</script>")
	// Should escape HTML entities
	if containsSubstring(result, "<script>") {
		t.Error("expected HTML to be escaped")
	}
	if !containsSubstring(result, "&lt;") || !containsSubstring(result, "&gt;") {
		t.Error("expected < and > to be escaped")
	}
}

func TestPlainTextToHTML_AmpersandEscaped(t *testing.T) {
	result := htmlsanitize.PlainTextToHTML("A & B")
	expected := "<p>A &amp; B</p>"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPrepareForDisplay_Empty(t *testing.T) {
	result := htmlsanitize.PrepareForDisplay("")
	if result != "" {
		t.Errorf("expected empty template.HTML, got %q", result)
	}
}

func TestPrepareForDisplay_PlainText(t *testing.T) {
	result := htmlsanitize.PrepareForDisplay("Hello, World!")
	expected := template.HTML("<p>Hello, World!</p>")
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestPrepareForDisplay_HTML(t *testing.T) {
	result := htmlsanitize.PrepareForDisplay("<p>Hello</p>")
	expected := template.HTML("<p>Hello</p>")
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestPrepareForDisplay_HTMLWithDangerousContent(t *testing.T) {
	result := htmlsanitize.PrepareForDisplay("<p>Hello</p><script>alert('xss')</script>")
	expected := template.HTML("<p>Hello</p>")
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestPrepareForDisplay_PlainTextWithNewlines(t *testing.T) {
	result := htmlsanitize.PrepareForDisplay("Line 1\nLine 2")
	expected := template.HTML("<p>Line 1<br>Line 2</p>")
	if result != expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestSanitize_RemovesOnError(t *testing.T) {
	input := `<img src="x" onerror="alert('xss')">`
	result := htmlsanitize.Sanitize(input)
	if containsSubstring(result, "onerror") {
		t.Error("expected onerror attribute to be removed")
	}
}

func TestSanitize_AllowsImages(t *testing.T) {
	input := `<img src="https://example.com/image.png" alt="Image">`
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, "src=") || !containsSubstring(result, "alt=") {
		t.Errorf("expected image preserved, got %q", result)
	}
}

func TestSanitize_RemovesDataURLInImage(t *testing.T) {
	// data: URLs in images could be used for attacks
	input := `<img src="data:text/html,<script>alert('xss')</script>">`
	result := htmlsanitize.Sanitize(input)
	// The src should be stripped since it's not an allowed protocol
	if containsSubstring(result, "data:text/html") {
		t.Error("expected data:text/html to be removed from image src")
	}
}

func TestSanitize_AllowsBreakTags(t *testing.T) {
	input := "Line 1<br>Line 2<br/>Line 3"
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, "<br") {
		t.Errorf("expected br tags preserved, got %q", result)
	}
}

func TestSanitize_AllowsHorizontalRule(t *testing.T) {
	input := "<p>Before</p><hr><p>After</p>"
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, "<hr") {
		t.Errorf("expected hr preserved, got %q", result)
	}
}

func TestSanitize_RemovesFormElements(t *testing.T) {
	input := `<form action="/submit"><input type="text" name="data"><button>Submit</button></form>`
	result := htmlsanitize.Sanitize(input)
	if containsSubstring(result, "<form") || containsSubstring(result, "<input") {
		t.Error("expected form elements to be removed")
	}
}

func TestSanitize_AllowsStyleOnTableElements(t *testing.T) {
	input := `<table style="width:100%"><tr><td style="text-align:center">Cell</td></tr></table>`
	result := htmlsanitize.Sanitize(input)
	if !containsSubstring(result, "style=") {
		t.Errorf("expected style attribute on table elements, got %q", result)
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
