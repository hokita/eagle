package app

import "strings"

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
