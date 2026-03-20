package service

import "testing"

func TestNormalizeCreateEmailUserInputDefaultsUsernameAndName(t *testing.T) {
	t.Parallel()

	got, err := normalizeCreateEmailUserInput(CreateEmailUserInput{
		Email:    " demo.user+test@example.com ",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("normalizeCreateEmailUserInput returned error: %v", err)
	}

	if got.Email != "demo.user+test@example.com" {
		t.Fatalf("unexpected email: %q", got.Email)
	}
	if got.Username != "demousertest" {
		t.Fatalf("unexpected username: %q", got.Username)
	}
	if got.Name != "demousertest" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
}

func TestNormalizeCreateEmailUserInputSanitizesExplicitUsername(t *testing.T) {
	t.Parallel()

	got, err := normalizeCreateEmailUserInput(CreateEmailUserInput{
		Email:    "demo@example.com",
		Password: "password123",
		Name:     "Demo User",
		Username: " demo-user_1 ",
	})
	if err != nil {
		t.Fatalf("normalizeCreateEmailUserInput returned error: %v", err)
	}

	if got.Username != "demouser_1" {
		t.Fatalf("unexpected username: %q", got.Username)
	}
	if got.Name != "Demo User" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
}

func TestNormalizeCreateEmailUserInputRejectsShortPassword(t *testing.T) {
	t.Parallel()

	if _, err := normalizeCreateEmailUserInput(CreateEmailUserInput{
		Email:    "demo@example.com",
		Password: "short",
	}); err == nil {
		t.Fatalf("expected error for short password")
	}
}
