package app

import "strings"

// ParseAllowedEmails splits a comma-separated list of emails (e.g. the
// ALLOWED_EMAILS env var), trimming whitespace and dropping empty entries.
func ParseAllowedEmails(raw string) []string {
	var emails []string
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			emails = append(emails, e)
		}
	}
	return emails
}
