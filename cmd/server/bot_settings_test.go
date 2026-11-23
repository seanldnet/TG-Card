package main

import "testing"

func TestNormalizeBotDefaultLanguage(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"en", "en"},
		{"EN", "en"},
		{"zh", "zh"},
		{"fr", "zh"},
		{"", "zh"},
	} {
		if got := normalizeBotDefaultLanguage(tc.input); got != tc.want {
			t.Errorf("normalizeBotDefaultLanguage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDecodeBotMenuSettingsDefaultLanguageCompatibility(t *testing.T) {
	if got := decodeBotMenuSettings(`{"ownerId":"123"}`).DefaultLanguage; got != "zh" {
		t.Fatalf("legacy Bot settings should default to Chinese, got %q", got)
	}
	if got := decodeBotMenuSettings(`{"defaultLanguage":"en"}`).DefaultLanguage; got != "en" {
		t.Fatalf("configured Bot default language should be preserved, got %q", got)
	}
	if got := decodeBotMenuSettings(`{"defaultLanguage":"unsupported"}`).DefaultLanguage; got != "zh" {
		t.Fatalf("unsupported Bot default language should fall back to Chinese, got %q", got)
	}
}
