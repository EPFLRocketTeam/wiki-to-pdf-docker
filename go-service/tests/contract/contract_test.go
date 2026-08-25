package contract_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	Type              string   `json:"type"`
	IgnoreFields      []string `json:"ignoreFields"`
	PDFByteDeltaMax   int      `json:"pdfByteDeltaMax"`
	PDFByteDeltaRatio float64  `json:"pdfByteDeltaRatio"`
}

func TestContractFixtures(t *testing.T) {
	if envOr("RUN_CONTRACT_TESTS", "") != "1" {
		t.Skip("set RUN_CONTRACT_TESTS=1 to run live contract replay tests")
	}

	pythonBase := envOr("PYTHON_BASE_URL", "http://localhost:8001")
	goBase := envOr("GO_BASE_URL", "http://localhost:8002")

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

			pythonVars := map[string]string{}
			goVars := map[string]string{}

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

				pyPath := resolveTemplate(step.Path, pythonVars)
				goPath := resolveTemplate(step.Path, goVars)

				pyReq := resolveAny(step.Request, pythonVars)
				goReq := resolveAny(step.Request, goVars)

				pyRequestBody, err := marshalRequest(pyReq)
				if err != nil {
					t.Fatalf("marshal python request for %s: %v", stepName, err)
				}
				goRequestBody, err := marshalRequest(goReq)
				if err != nil {
					t.Fatalf("marshal go request for %s: %v", stepName, err)
				}

				pyStatus, pyHeaders, pyBody, err := execute(client, step.Method, pythonBase+pyPath, pyRequestBody)
				if err != nil {
					t.Fatalf("python request failed at %s: %v", stepName, err)
				}

				goStatus, goHeaders, goBody, err := execute(client, step.Method, goBase+goPath, goRequestBody)
				if err != nil {
					t.Fatalf("go request failed at %s: %v", stepName, err)
				}

				if pyStatus != goStatus {
					t.Fatalf("status mismatch at %s for %s: python=%d go=%d", stepName, step.Path, pyStatus, goStatus)
				}

				switch step.Compare.Type {
				case "json":
					if err := compareJSONBodies(pyBody, goBody, step.Compare.IgnoreFields); err != nil {
						t.Fatalf("json diff at %s: %v\npython=%s\ngo=%s", stepName, err, string(pyBody), string(goBody))
					}
				case "pdf":
					if err := comparePDFBodies(pyHeaders, pyBody, goHeaders, goBody, step.Compare); err != nil {
						t.Fatalf("pdf diff at %s: %v", stepName, err)
					}
				case "zip":
					if err := compareZIPBodies(pyHeaders, pyBody, goHeaders, goBody, step.Compare); err != nil {
						t.Fatalf("zip diff at %s: %v", stepName, err)
					}
				default:
					t.Fatalf("unsupported compare type at %s: %s", stepName, step.Compare.Type)
				}

				for varName, fieldPath := range step.SaveFields {
					pyVal, err := extractJSONField(pyBody, fieldPath)
					if err == nil {
						pythonVars[varName] = pyVal
					}
					goVal, err := extractJSONField(goBody, fieldPath)
					if err == nil {
						goVars[varName] = goVal
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
			if fx.Steps[i].Compare.PDFByteDeltaMax == 0 {
				fx.Steps[i].Compare.PDFByteDeltaMax = 2048
			}
			if fx.Steps[i].Compare.PDFByteDeltaRatio == 0 {
				fx.Steps[i].Compare.PDFByteDeltaRatio = 0.05
			}
		}
		if fx.Compare.PDFByteDeltaMax == 0 {
			fx.Compare.PDFByteDeltaMax = 2048
		}
		if fx.Compare.PDFByteDeltaRatio == 0 {
			fx.Compare.PDFByteDeltaRatio = 0.05
		}
		out = append(out, fx)
	}
	return out, nil
}

func compareJSONBodies(pyBody, goBody []byte, ignoreFields []string) error {
	var py any
	if err := json.Unmarshal(pyBody, &py); err != nil {
		return fmt.Errorf("python response not json: %w", err)
	}
	var goResp any
	if err := json.Unmarshal(goBody, &goResp); err != nil {
		return fmt.Errorf("go response not json: %w", err)
	}

	py = stripFields(py, ignoreFields)
	goResp = stripFields(goResp, ignoreFields)

	if !deepEqualJSON(py, goResp) {
		pyNorm, _ := json.Marshal(py)
		goNorm, _ := json.Marshal(goResp)
		return fmt.Errorf("normalized JSON mismatch: python=%s go=%s", pyNorm, goNorm)
	}
	return nil
}

func compareZIPBodies(pyHeaders http.Header, pyBody []byte, goHeaders http.Header, goBody []byte, rule compareRule) error {
	if !strings.HasPrefix(pyHeaders.Get("Content-Type"), "application/zip") {
		return fmt.Errorf("python content type is not application/zip: %s", pyHeaders.Get("Content-Type"))
	}
	if !strings.HasPrefix(goHeaders.Get("Content-Type"), "application/zip") {
		return fmt.Errorf("go content type is not application/zip: %s", goHeaders.Get("Content-Type"))
	}
	if len(pyBody) < 4 || string(pyBody[:2]) != "PK" {
		return fmt.Errorf("python body does not look like ZIP")
	}
	if len(goBody) < 4 || string(goBody[:2]) != "PK" {
		return fmt.Errorf("go body does not look like ZIP")
	}

	pySize := len(pyBody)
	goSize := len(goBody)
	delta := int(math.Abs(float64(pySize - goSize)))

	ratioBase := pySize
	if ratioBase == 0 {
		ratioBase = 1
	}
	ratio := float64(delta) / float64(ratioBase)

	if delta > rule.PDFByteDeltaMax && ratio > rule.PDFByteDeltaRatio {
		return fmt.Errorf("zip size mismatch: python=%d go=%d delta=%d ratio=%.4f", pySize, goSize, delta, ratio)
	}

	return nil
}

func stripFields(v any, ignoreFields []string) any {
	ignore := map[string]struct{}{}
	for _, k := range ignoreFields {
		ignore[k] = struct{}{}
	}

	var walk func(any) any
	walk = func(node any) any {
		switch n := node.(type) {
		case map[string]any:
			out := make(map[string]any, len(n))
			for k, v := range n {
				if _, skip := ignore[k]; skip {
					continue
				}
				out[k] = walk(v)
			}
			return out
		case []any:
			out := make([]any, len(n))
			for i := range n {
				out[i] = walk(n[i])
			}
			return out
		default:
			return n
		}
	}

	return walk(v)
}

func deepEqualJSON(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}

func comparePDFBodies(pyHeaders http.Header, pyBody []byte, goHeaders http.Header, goBody []byte, rule compareRule) error {
	if !strings.HasPrefix(pyHeaders.Get("Content-Type"), "application/pdf") {
		return fmt.Errorf("python content type is not application/pdf: %s", pyHeaders.Get("Content-Type"))
	}
	if !strings.HasPrefix(goHeaders.Get("Content-Type"), "application/pdf") {
		return fmt.Errorf("go content type is not application/pdf: %s", goHeaders.Get("Content-Type"))
	}
	if !bytes.HasPrefix(pyBody, []byte("%PDF")) {
		return fmt.Errorf("python body does not look like a PDF")
	}
	if !bytes.HasPrefix(goBody, []byte("%PDF")) {
		return fmt.Errorf("go body does not look like a PDF")
	}

	pySize := len(pyBody)
	goSize := len(goBody)
	delta := int(math.Abs(float64(pySize - goSize)))

	ratioBase := pySize
	if ratioBase == 0 {
		ratioBase = 1
	}
	ratio := float64(delta) / float64(ratioBase)

	if delta > rule.PDFByteDeltaMax && ratio > rule.PDFByteDeltaRatio {
		return fmt.Errorf(
			"pdf size mismatch: python=%d go=%d delta=%d ratio=%.4f maxDelta=%d maxRatio=%.4f pySha=%s goSha=%s",
			pySize,
			goSize,
			delta,
			ratio,
			rule.PDFByteDeltaMax,
			rule.PDFByteDeltaRatio,
			sha256Hex(pyBody),
			sha256Hex(goBody),
		)
	}

	return nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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
