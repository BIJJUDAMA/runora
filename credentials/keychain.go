package credentials

import (
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	// ServiceName is the application credential namespace registered with the OS vault.
	ServiceName = "runora"

	// ProviderGitHub is the account key for GitHub Personal Access Tokens.
	ProviderGitHub = "github"

	// ProviderHuggingFace is the account key for Hugging Face API Tokens.
	ProviderHuggingFace = "huggingface"
)

// Set stores an API token for the specified provider in the OS vault.
func Set(provider string, token string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		// Empty token deletes credential from keyring
		return Delete(provider)
	}
	return keyring.Set(ServiceName, provider, trimmed)
}

// Get retrieves an API token for the specified provider from the OS vault.
// Returns an empty string and nil error if no credential exists.
func Get(provider string) (string, error) {
	val, err := keyring.Get(ServiceName, provider)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(val), nil
}

// Delete removes an API token for the specified provider from the OS vault.
func Delete(provider string) error {
	err := keyring.Delete(ServiceName, provider)
	if err != nil && err == keyring.ErrNotFound {
		return nil
	}
	return err
}

// MockInit initializes an in-memory mock keyring for testing.
func MockInit() {
	keyring.MockInit()
}
