package miniapp

import (
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestValidateInitDataAcceptsSignedOwner(t *testing.T) {
	raw := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0))

	viewer, err := ValidateInitData(raw, "bot-token", 42)
	if err != nil {
		t.Fatalf("ValidateInitData() error = %v", err)
	}

	if viewer.ID != 42 {
		t.Fatalf("viewer.ID = %d, want %d", viewer.ID, 42)
	}

	if viewer.Username != "jason" {
		t.Fatalf("viewer.Username = %q, want %q", viewer.Username, "jason")
	}
}

func TestValidateInitDataRejectsBadHash(t *testing.T) {
	raw := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0)) + "broken"

	if _, err := ValidateInitData(raw, "bot-token", 42); err == nil {
		t.Fatal("ValidateInitData() error = nil, want error")
	}
}

func TestValidateInitDataRejectsNonOwner(t *testing.T) {
	raw := signedInitData(t, "bot-token", 42, "jason", time.Unix(1_700_000_000, 0))

	if _, err := ValidateInitData(raw, "bot-token", 7); err == nil {
		t.Fatal("ValidateInitData() error = nil, want error")
	}
}

func signedInitData(t *testing.T, botToken string, userID int64, username string, authDate time.Time) string {
	t.Helper()

	values := url.Values{}
	values.Set("auth_date", strconv.FormatInt(authDate.Unix(), 10))
	values.Set("user", `{"id":`+strconv.FormatInt(userID, 10)+`,"username":"`+username+`"}`)
	values.Set("hash", signInitData(botToken, values))
	return values.Encode()
}
