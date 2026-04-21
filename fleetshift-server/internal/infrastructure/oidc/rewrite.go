package oidc

import (
	"net/url"
	"strings"
)

func rewriteLocalhost(rawURL, containerHost string) string {
	if containerHost == "" {
		return rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	host := strings.Split(u.Host, ":")[0]
	if host != "localhost" && host != "127.0.0.1" {
		return rawURL
	}

	port := u.Port()
	if port != "" {
		u.Host = containerHost + ":" + port
	} else {
		u.Host = containerHost
	}
	return u.String()
}
