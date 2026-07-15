package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func cleanAadhar(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isValidAadhar(a string) bool {
	if len(a) != 12 {
		return false
	}
	for _, r := range a {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// cleanPhone strips everything but digits, then trims a leading
// country/trunk prefix (+91 / 91 / 0) so numbers typed in different
// formats ("+91 98765 43210", "09876543210", "9876543210") all
// normalize to the same plain 10-digit form.
func cleanPhone(raw string) string {
	digits := cleanAadhar(raw)
	switch {
	case len(digits) == 12 && strings.HasPrefix(digits, "91"):
		digits = digits[2:]
	case len(digits) == 11 && strings.HasPrefix(digits, "0"):
		digits = digits[1:]
	}
	return digits
}

// isValidPhone checks for a plain 10-digit Indian mobile number
// (starts 6-9, per the Indian numbering plan).
func isValidPhone(p string) bool {
	return len(p) == 10 && p[0] >= '6' && p[0] <= '9'
}
