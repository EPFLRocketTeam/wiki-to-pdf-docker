package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/epflrocketteam/wiki-to-pdf-go/internal/model"
)

type Client interface {
	FetchContents(ctx context.Context, pages []model.WikiPage, graphQLURL, token string) ([]model.WikiContent, error)
	GetAccessToken(ctx context.Context, req model.GetAccessTokenRequest) (string, error)
}

type client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) Client {
	return &client{http: &http.Client{Timeout: timeout}}
}

func ParseRocketURLs(urls []string) []model.WikiPage {
	supported := map[string]struct{}{"en": {}, "fr": {}, "de": {}, "it": {}}
	pages := make([]model.WikiPage, 0, len(urls))

	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		locale := "en"
		if len(segments) > 0 {
			if _, ok := supported[segments[0]]; ok {
				locale = segments[0]
				segments = segments[1:]
			}
		}
		path := "home"
		if len(segments) > 0 {
			path = strings.Join(segments, "/")
		}
		pages = append(pages, model.WikiPage{Path: path, Locale: locale})
	}

	return pages
}

func (c *client) FetchContents(ctx context.Context, pages []model.WikiPage, graphQLURL, token string) ([]model.WikiContent, error) {
	out := make([]model.WikiContent, 0, len(pages))
	for _, p := range pages {
		query := fmt.Sprintf(`{ pages { singleByPath(path: %q, locale: %q) { path title createdAt updatedAt authorName content } } }`, p.Path, p.Locale)
		payload := map[string]string{"query": query}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLURL, bytes.NewReader(body))
		if err != nil {
			out = append(out, model.WikiContent{Path: p.Path, Error: err.Error()})
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			out = append(out, model.WikiContent{Path: p.Path, Error: err.Error()})
			continue
		}

		var decoded struct {
			Data struct {
				Pages struct {
					SingleByPath model.WikiContent `json:"singleByPath"`
				} `json:"pages"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}

		err = json.NewDecoder(resp.Body).Decode(&decoded)
		_ = resp.Body.Close()
		if err != nil {
			out = append(out, model.WikiContent{Path: p.Path, Error: err.Error()})
			continue
		}

		if len(decoded.Errors) > 0 {
			out = append(out, model.WikiContent{Path: p.Path, Error: "graphql returned errors"})
			continue
		}
		out = append(out, decoded.Data.Pages.SingleByPath)
	}

	return out, nil
}

func (c *client) GetAccessToken(ctx context.Context, in model.GetAccessTokenRequest) (string, error) {
	query := fmt.Sprintf(`mutation { authentication { login(username: %q, password: %q, strategy: "local") { jwt } } }`, in.Username, in.Password)
	payload := map[string]string{"query": query}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, in.EndpointURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var decoded struct {
		Data struct {
			Authentication struct {
				Login struct {
					JWT string `json:"jwt"`
				} `json:"login"`
			} `json:"authentication"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if decoded.Data.Authentication.Login.JWT == "" {
		return "", fmt.Errorf("invalid GraphQL response")
	}
	return decoded.Data.Authentication.Login.JWT, nil
}
