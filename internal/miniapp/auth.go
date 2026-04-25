package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func ValidateInitData(raw string, botToken string, ownerID int64) (Viewer, error) {
	if strings.TrimSpace(raw) == "" {
		return Viewer{}, fmt.Errorf("missing init data")
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return Viewer{}, fmt.Errorf("parse init data: %w", err)
	}

	hash := strings.TrimSpace(values.Get("hash"))
	if hash == "" {
		return Viewer{}, fmt.Errorf("missing init data hash")
	}

	if _, err := strconv.ParseInt(values.Get("auth_date"), 10, 64); err != nil {
		return Viewer{}, fmt.Errorf("invalid auth_date")
	}

	if expected := signInitData(botToken, values); !hmac.Equal([]byte(expected), []byte(hash)) {
		return Viewer{}, fmt.Errorf("invalid init data hash")
	}

	var viewer Viewer
	if err := json.Unmarshal([]byte(values.Get("user")), &viewer); err != nil {
		return Viewer{}, fmt.Errorf("invalid user payload: %w", err)
	}

	if ownerID > 0 && viewer.ID != ownerID {
		return Viewer{}, fmt.Errorf("viewer %d is not allowed", viewer.ID)
	}

	return viewer, nil
}

func signInitData(botToken string, values url.Values) string {
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secretMAC.Write([]byte(botToken))
	secret := secretMAC.Sum(nil)

	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values.Get(key))
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}
