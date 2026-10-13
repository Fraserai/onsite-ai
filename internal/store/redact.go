package store

import (
	"errors"
	"net/url"
	"strings"
)

// redact strips the database password out of an error message. Driver errors
// sometimes echo the connection string, and secrets must never reach the logs.
func redact(err error, databaseURL string) error {
	if err == nil {
		return nil
	}
	u, parseErr := url.Parse(databaseURL)
	if parseErr != nil || u.User == nil {
		return err
	}
	password, ok := u.User.Password()
	if !ok || password == "" {
		return err
	}
	msg := err.Error()
	clean := msg
	for _, secret := range []string{password, url.QueryEscape(password), url.PathEscape(password)} {
		clean = strings.ReplaceAll(clean, secret, "[redacted]")
	}
	if clean == msg {
		return err
	}
	return errors.New(clean)
}
