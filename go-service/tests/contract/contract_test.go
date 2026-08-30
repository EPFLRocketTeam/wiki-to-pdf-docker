package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	Name             string         `json:"name"`
	Method           string         `json:"method"`
	Path             string         `json:"path"`
	Request          map[string]any `json:"request"`
	Compare          compareRule    `json:"compare"`
	SkipIfMissingEnv []string       `json:"skipIfMissingEnv"`
	Steps            []scenarioStep `json:"steps"`
}

type scenarioStep struct {
	Name       string            `json:"name"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Request    map[string]any    `json:"request"`
	Compare    compareRule       `json:"compare"`
	SaveFields map[string]string `json:"saveFields"`
}

type compareRule struct {
	Type string `json:"type"`
}

func TestContractFixtures(t *testing.T) {
	if envOr("RUN_CONTRACT_TESTS", "") != "1" {
		t.Skip("set RUN_CONTRACT_TESTS=1 to run live contract replay tests")
	}

	baseURL := envOr("GO_BASE_URL", "http://localhost:8000")

	fixtures, err := loadFixtures("fixtures")
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures found")
	}

	client := &http.Client{Timeout: 180 * time.Second}

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			for _, envName := range fx.SkipIfMissingEnv {
				if strings.TrimSpace(os.Getenv(envName)) == "" {
					t.Skipf("skipping %s because %s is not set", fx.Name, envName)
				}
			}

			steps := fx.Steps
			if len(steps) == 0 {
				steps = []scenarioStep{{
					Name:    fx.Name,
					Method:  fx.Method,
					Path:    fx.Path,
					Request: fx.Request,
					Compare: fx.Compare,
				}}
			}

			vars := map[string]string{}

			for idx, step := range steps {
				stepName := step.Name
				if stepName == "" {
					stepName = fmt.Sprintf("step-%d", idx+1)
				}
				if step.Method == "" {
					step.Method = http.MethodPost
				}
				if step.Compare.Type == "" {
					step.Compare.Type = "json"
				}

				path := resolveTemplate(step.Path, vars)
				req := resolveAny(step.Request, vars)

				requestBody, err := marshalRequest(req)
				if err != nil {
					t.Fatalf("marshal request for %s: %v", stepName, err)
				}

				status, headers, body, err := execute(client, step.Method, baseURL+path, requestBody)
				if err != nil {
					t.Fatalf("request failed at %s: %v", stepName, err)
				}

				if status < 200 || status >= 300 {
					t.Fatalf("unexpected status at %s for %s: %d", stepName, step.Path, status)
				}

				switch step.Compare.Type {
				case "json":
					if err := ensureJSONBody(body); err != nil {
						t.Fatalf("json validation failed at %s: %v\nbody=%s", stepName, err, string(body))
					}
				case "pdf":
					if err := ensurePDFBody(headers, body); err != nil {
						t.Fatalf("pdf validation failed at %s: %v", stepName, err)
					}
				case "zip":
					if err := ensureZIPBody(headers, body); err != nil {
						t.Fatalf("zip validation failed at %s: %v", stepName, err)
					}
				default:
					t.Fatalf("unsupported compare type at %s: %s", stepName, step.Compare.Type)
				}

				for varName, fieldPath := range step.SaveFields {
					val, err := extractJSONField(body, fieldPath)
					if err == nil {
						vars[varName] = val
					}
				}
			}
		})
	}
}

func marshalRequest(req any) ([]byte, error) {
	if req == nil {
		return nil, nil
	}
	return json.Marshal(req)
}

func execute(client *http.Client, method, url string, body []byte) (int, http.Header, []byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header.Clone(), b, nil
}

func loadFixtures(dir string) ([]fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)

	out := make([]fixture, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var fx fixture
		if err := json.Unmarshal(data, &fx); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if fx.Name == "" {
			fx.Name = strings.TrimSuffix(filepath.Base(file), ".json")
		}
		if fx.Method == "" {
			fx.Method = http.MethodPost
		}
		for i := range fx.Steps {
			if fx.Steps[i].Method == "" {
				fx.Steps[i].Method = http.MethodPost
			}
			if fx.Steps[i].Compare.Type == "" {
				fx.Steps[i].Compare.Type = "json"
			}
		}
		out = append(out, fx)
	}
	return out, nil
}

func ensureJSONBody(body []byte) error {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("response is not json: %w", err)
	}
	return nil
}

func ensureZIPBody(headers http.Header, body []byte) error {
	if !strings.HasPrefix(headers.Get("Content-Type"), "application/zip") {
		return fmt.Errorf("content type is not application/zip: %s", headers.Get("Content-Type"))
	}
	if len(body) < 4 || string(body[:2]) != "PK" {
		return fmt.Errorf("body does not look like ZIP")
	}
	return nil
}

func ensurePDFBody(headers http.Header, body []byte) error {
	if !strings.HasPrefix(headers.Get("Content-Type"), "application/pdf") {
		return fmt.Errorf("content type is not application/pdf: %s", headers.Get("Content-Type"))
	}
	if !bytes.HasPrefix(body, []byte("%PDF")) {
		return fmt.Errorf("body does not look like a PDF")
	}
	return nil
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

var templateRe = regexp.MustCompile(`\{\{(env|var):([A-Za-z0-9_\-]+)\}\}`)

func resolveTemplate(in string, vars map[string]string) string {
	return templateRe.ReplaceAllStringFunc(in, func(m string) string {
		parts := templateRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		kind := parts[1]
		name := parts[2]
		if kind == "env" {
			return os.Getenv(name)
		}
		if v, ok := vars[name]; ok {
			return v
		}
		return ""
	})
}

func resolveAny(in any, vars map[string]string) any {
	switch v := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = resolveAny(child, vars)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = resolveAny(v[i], vars)
		}
		return out
	case string:
		return resolveTemplate(v, vars)
	default:
		return in
	}
}

func extractJSONField(body []byte, fieldPath string) (string, error) {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	cur := data
	for _, segment := range strings.Split(fieldPath, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field path invalid at %s", segment)
		}
		next, ok := obj[segment]
		if !ok {
			return "", fmt.Errorf("field %s not found", segment)
		}
		cur = next
	}
	switch val := cur.(type) {
	case string:
		return val, nil
	case float64:
		return fmt.Sprintf("%v", val), nil
	case bool:
		if val {
			return "true", nil
		}
		return "false", nil
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
