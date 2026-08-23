package credentials

import (
	"testing"
)

func TestCredentialsLifecycle(t *testing.T) {
	MockInit()

	// 1. Initial Get should return empty string without error
	val, err := Get(ProviderGitHub)
	if err != nil {
		t.Fatalf("expected nil error on empty key, got: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for non-existent key, got: %q", val)
	}

	// 2. Set GitHub token
	testToken := "ghp_1234567890abcdef"
	if err := Set(ProviderGitHub, testToken); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 3. Get GitHub token
	loaded, err := Get(ProviderGitHub)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if loaded != testToken {
		t.Errorf("loaded token mismatch, got %q, expected %q", loaded, testToken)
	}

	// 4. Set HuggingFace token
	hfToken := "hf_abcdef1234567890"
	if err := Set(ProviderHuggingFace, hfToken); err != nil {
		t.Fatalf("Set HF token failed: %v", err)
	}
	loadedHF, err := Get(ProviderHuggingFace)
	if err != nil {
		t.Fatalf("Get HF token failed: %v", err)
	}
	if loadedHF != hfToken {
		t.Errorf("loaded HF token mismatch, got %q, expected %q", loadedHF, hfToken)
	}

	// 5. Setting empty string should delete
	if err := Set(ProviderGitHub, ""); err != nil {
		t.Fatalf("Set empty string failed: %v", err)
	}
	cleared, err := Get(ProviderGitHub)
	if err != nil {
		t.Fatalf("Get after clear failed: %v", err)
	}
	if cleared != "" {
		t.Errorf("expected empty token after clearing, got: %q", cleared)
	}

	// 6. Delete HuggingFace token
	if err := Delete(ProviderHuggingFace); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	clearedHF, err := Get(ProviderHuggingFace)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if clearedHF != "" {
		t.Errorf("expected empty HF token after delete, got: %q", clearedHF)
	}

	// 7. Deleting already non-existent key should not return error
	if err := Delete("non_existent_provider"); err != nil {
		t.Errorf("expected nil error on deleting non-existent provider, got: %v", err)
	}
}
