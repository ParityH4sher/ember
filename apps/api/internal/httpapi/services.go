package httpapi

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// ServiceLink is a public browser destination, never an upstream API credential.
type ServiceLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func ParseServiceLinks(raw string) ([]ServiceLink, error) {
	links := []ServiceLink{}
	if raw == "" {
		return links, nil
	}
	if err := json.Unmarshal([]byte(raw), &links); err != nil || links == nil {
		return nil, errors.New("EMBER_SERVICE_LINKS must be a JSON array of service links")
	}
	for _, link := range links {
		u, err := url.Parse(link.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") ||
			u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(link.URL, `\`) || strings.TrimSpace(link.Name) == "" {
			return nil, errors.New("service links require a name and an absolute HTTP(S) URL without credentials, query or fragment")
		}
	}
	return links, nil
}
