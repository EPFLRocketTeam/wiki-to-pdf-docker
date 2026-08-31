package conversion

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/epflrocketteam/wiki-to-pdf-go/internal/config"
	"github.com/epflrocketteam/wiki-to-pdf-go/internal/model"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type Converter struct {
	cfg config.Config

	httpClient *http.Client
}

type ConvertResult struct {
	Latex     string
	SessionID string
	ZipPath   string
}

const bundledBaseTemplatePath = "/app/latex_templates/base.tex"

func NewConverter(cfg config.Config) *Converter {
	return &Converter{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Converter) ConvertAndPackage(ctx context.Context, req model.ConvertRequest) (ConvertResult, error) {
	filtered := filterText(req.Markdown)

	workingDir, err := os.MkdirTemp("", "wiki-to-pdf-go-*")
	if err != nil {
		return ConvertResult{}, err
	}
	defer os.RemoveAll(workingDir)

	mdPath := filepath.Join(workingDir, "source.md")
	if err := os.WriteFile(mdPath, []byte(filtered), 0o644); err != nil {
		return ConvertResult{}, err
	}

	metadata := map[string]any{
		"author":          req.Author,
		"date":            req.Date,
		"title":           req.Title,
		"documentId":      req.DocumentID,
		"footerText":      req.FooterText,
		"lineNumbers":     req.LineNumbersEnabled,
		"assetsDirectory": c.cfg.AssetsTemplateDir,
		"imageBaseUrl":    req.ImageBaseURL,
	}

	metaPath := filepath.Join(workingDir, "metadata.yaml")
	metaBytes, _ := yaml.Marshal(metadata)
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		return ConvertResult{}, err
	}

	templateVars := map[string]map[string]string{
		"default":     {},
		"generic":     {"backgroundImage": "ert_title-page.png"},
		"competition": {"backgroundImage": "fh2_title-page.png", "rheadImage": "fh2_patch.png"},
		"hyperion":    {"backgroundImage": "h_title-page.png", "rheadImage": "h_patch.png"},
		"icarus":      {"backgroundImage": "i_title-page.png", "rheadImage": "i_patch.png"},
		"management":  {"backgroundImage": "m_title-page.png"},
		"space-race":  {"backgroundImage": "s_title-page.png", "rheadImage": "s_patch.png"},
	}

	args := []string{
		"--standalone",
		"--from", "markdown",
		"--to", "latex",
		"--lua-filter", c.cfg.LuaFilterPath,
		"--variable", "code-block-environment=verbatim",
		"--template", c.baseTemplatePath(),
		"--metadata-file", metaPath,
		mdPath,
	}
	for k, v := range templateVars[req.Template] {
		args = append(args, "--variable", fmt.Sprintf("%s=%s", k, v))
	}

	cmd := exec.CommandContext(ctx, c.cfg.PandocBinary, args...)
	cmd.Dir = workingDir
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ConvertResult{}, fmt.Errorf("pandoc failed: %w; stderr=%s", err, stderr.String())
	}

	latex := out.String()

	projectDir := filepath.Join(workingDir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return ConvertResult{}, err
	}

	mainTexPath := filepath.Join(projectDir, "main.tex")
	safeLatex := removeEmojis(latex)
	if err := os.WriteFile(mainTexPath, []byte(safeLatex), 0o644); err != nil {
		return ConvertResult{}, err
	}

	mapping, err := c.packageProvidedImages(safeLatex, projectDir, req.ImageBaseURL, req.Images)
	if err != nil {
		return ConvertResult{}, err
	}
	mapping, err = c.collectAssets(ctx, safeLatex, projectDir, req.ImageAuthToken, mapping)
	if err != nil {
		return ConvertResult{}, err
	}
	if err := c.validateWikiImageAssets(safeLatex, mapping); err != nil {
		return ConvertResult{}, err
	}
	if err := rewriteAssetReferences(mainTexPath, mapping); err != nil {
		return ConvertResult{}, err
	}
	packagedLatex, err := os.ReadFile(mainTexPath)
	if err != nil {
		return ConvertResult{}, err
	}

	sessionID := uuid.NewString()
	zipPath := filepath.Join(os.TempDir(), sessionID+".zip")
	if err := zipDirectory(projectDir, zipPath); err != nil {
		return ConvertResult{}, err
	}

	return ConvertResult{Latex: string(packagedLatex), SessionID: sessionID, ZipPath: zipPath}, nil
}

// baseTemplatePath tolerates a legacy deployment override such as
// BASE_TEMPLATE_PATH=base.tex. Pandoc interprets that value as the name of an
// internal template, while the container bundles the actual template at the
// absolute path below.
func (c *Converter) baseTemplatePath() string {
	if fileExists(c.cfg.BaseTemplatePath) {
		return c.cfg.BaseTemplatePath
	}
	if fileExists(bundledBaseTemplatePath) {
		return bundledBaseTemplatePath
	}
	return c.cfg.BaseTemplatePath
}

func (c *Converter) GeneratePDF(ctx context.Context, latexCode string, assetsZipData []byte) ([]byte, error) {
	workingDir, err := os.MkdirTemp("", "wiki-to-pdf-compile-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workingDir)
	if len(assetsZipData) != 0 {
		if err := unzipData(assetsZipData, workingDir); err != nil {
			return nil, fmt.Errorf("extract conversion assets: %w", err)
		}
	}

	texPath := filepath.Join(workingDir, "document.tex")
	pdfPath := filepath.Join(workingDir, "document.pdf")
	latexCode = repairLegacyTemplateMacros(latexCode)

	if err := os.WriteFile(texPath, []byte(latexCode), 0o644); err != nil {
		return nil, err
	}
	if err := c.materializeTemplateAssetFallbacks(latexCode, workingDir); err != nil {
		return nil, fmt.Errorf("prepare template assets: %w", err)
	}

	if err := addDraftToDocumentClass(texPath); err != nil {
		return nil, err
	}
	if err := runLuaLatex(ctx, c.cfg.LuaLatexBinary, texPath, workingDir); err != nil {
		return nil, fmt.Errorf("first compilation failed: %w", err)
	}

	if err := removeDraftFromDocumentClass(texPath); err != nil {
		return nil, err
	}
	if err := runLuaLatex(ctx, c.cfg.LuaLatexBinary, texPath, workingDir); err != nil {
		return nil, fmt.Errorf("second compilation failed: %w", err)
	}

	b, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func runLuaLatex(ctx context.Context, binary, texPath, outputDir string) error {
	cmd := exec.CommandContext(ctx, binary, "-interaction=nonstopmode", "-halt-on-error", "-shell-escape", "-output-directory", outputDir, texPath)
	cmd.Dir = outputDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (c *Converter) collectAssets(ctx context.Context, latexText, projectDir, authToken string, mapping map[string]string) (map[string]string, error) {
	imgRe := regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
	inputRe := regexp.MustCompile(`\\(?:input|include)\{([^}]+)\}`)
	allRefs := append(imgRe.FindAllStringSubmatch(latexText, -1), inputRe.FindAllStringSubmatch(latexText, -1)...)

	if mapping == nil {
		mapping = map[string]string{}
	}
	for _, match := range allRefs {
		if len(match) < 2 {
			continue
		}
		orig := strings.TrimSpace(match[1])
		if orig == "" {
			continue
		}
		if mapping[orig] != "" {
			continue
		}
		unescaped := strings.ReplaceAll(orig, `\ `, " ")

		if strings.HasPrefix(unescaped, "http://") || strings.HasPrefix(unescaped, "https://") {
			rel, err := c.downloadAsset(ctx, projectDir, unescaped, authToken)
			if err != nil {
				return nil, fmt.Errorf("download image %q: %w", unescaped, err)
			}
			mapping[orig] = rel
			continue
		}

		if strings.HasPrefix(unescaped, `\assetsDirectory/`) {
			// Bundled template images are always available in the runtime image.
			// Keep this reference intact so PDF compilation does not depend on the
			// conversion ZIP being available in a later request.
			continue
		}

		src := c.resolveAssetPath(unescaped)
		if src == "" {
			continue
		}

		rel := assetRelativePath(src, filepath.Base(src))
		dst := filepath.Join(projectDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := copyFile(src, dst); err != nil {
			continue
		}
		mapping[orig] = rel
	}

	return mapping, nil
}

// packageProvidedImages writes images sent in the conversion request into the
// project and maps the corresponding generated LaTeX references to them. This
// lets callers supply protected Wiki.js images directly, without the service
// having to authenticate to or download from their wiki.
func (c *Converter) packageProvidedImages(latexText, projectDir, imageBaseURL string, images []model.ImageAsset) (map[string]string, error) {
	provided := make(map[string]model.ImageAsset, len(images))
	for _, image := range images {
		key, err := canonicalImagePath(image.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid supplied image path %q: %w", image.Path, err)
		}
		if _, exists := provided[key]; exists {
			return nil, fmt.Errorf("duplicate supplied image path %q", image.Path)
		}
		provided[key] = image
	}

	mapping := map[string]string{}
	imgRe := regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
	for _, match := range imgRe.FindAllStringSubmatch(latexText, -1) {
		if len(match) < 2 {
			continue
		}
		orig := strings.TrimSpace(match[1])
		image, ok := providedImageForReference(provided, orig, imageBaseURL)
		if !ok {
			continue
		}

		key, _ := canonicalImagePath(image.Path)
		rel := assetRelativePath("provided:"+key, filepath.Base(key))
		dst := filepath.Join(projectDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, image.Content, 0o644); err != nil {
			return nil, err
		}
		mapping[orig] = rel
	}
	return mapping, nil
}

func providedImageForReference(provided map[string]model.ImageAsset, reference, imageBaseURL string) (model.ImageAsset, bool) {
	for _, candidate := range imageReferenceCandidates(reference, imageBaseURL) {
		if image, ok := provided[candidate]; ok {
			return image, true
		}
	}
	return model.ImageAsset{}, false
}

func imageReferenceCandidates(reference, imageBaseURL string) []string {
	reference = strings.ReplaceAll(strings.TrimSpace(reference), `\ `, " ")
	candidates := []string{reference}
	if strings.HasPrefix(reference, "/app/ert_wiki/") {
		candidates = append(candidates, strings.TrimPrefix(reference, "/app/ert_wiki/"))
	}
	if parsed, err := url.Parse(reference); err == nil && parsed.IsAbs() {
		candidates = append(candidates, parsed.Path)
	}
	if base, err := url.Parse(imageBaseURL); err == nil && base.IsAbs() {
		if parsed, err := url.Parse(reference); err == nil && parsed.IsAbs() && parsed.Scheme == base.Scheme && parsed.Host == base.Host {
			candidates = append(candidates, strings.TrimPrefix(parsed.Path, base.Path))
		}
	}

	keys := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if key, err := canonicalImagePath(candidate); err == nil {
			keys = append(keys, key)
		}
	}
	return keys
}

func canonicalImagePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.IsAbs() {
		raw = parsed.Path
	}
	raw = strings.TrimPrefix(strings.ReplaceAll(raw, "\\", "/"), "/")
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", fmt.Errorf("path traversal is not allowed")
		}
	}
	clean := pathpkg.Clean(raw)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("path is empty")
	}
	return clean, nil
}

func (c *Converter) downloadAsset(ctx context.Context, projectDir, rawURL, authToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if authToken != "" {
		if strings.HasPrefix(strings.ToLower(authToken), "bearer ") {
			req.Header.Set("Authorization", authToken)
		} else {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bad status %d", resp.StatusCode)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	filename := filepath.Base(parsedURL.Path)
	if filename == "." || filename == "/" || filename == "" {
		filename = "downloaded-asset"
	}
	cleanPath := assetRelativePath(rawURL, filename)

	dst := filepath.Join(projectDir, cleanPath)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return cleanPath, nil
}

func assetRelativePath(identity, filename string) string {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))[:16]
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	return filepath.Join("assets", digest+"_"+filename)
}

func (c *Converter) validateWikiImageAssets(latexText string, mapping map[string]string) error {
	imgRe := regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
	for _, match := range imgRe.FindAllStringSubmatch(latexText, -1) {
		if len(match) < 2 {
			continue
		}
		ref := strings.TrimSpace(match[1])
		if mapping[ref] != "" || strings.HasPrefix(ref, `\assetsDirectory/`) {
			continue
		}
		if strings.HasPrefix(ref, "/app/ert_wiki/") {
			return fmt.Errorf("Wiki.js image %q is unavailable locally; include it in the convert request images array", ref)
		}
		if filepath.IsAbs(ref) {
			return fmt.Errorf("generated image %q could not be packaged; check the diagram source and required conversion tool", ref)
		}
	}
	return nil
}

// materializeTemplateAssetFallbacks restores bundled template images by their
// original filename when generated LaTeX is compiled without conversion assets.
func (c *Converter) materializeTemplateAssetFallbacks(latexCode, workingDir string) error {
	assetRef := regexp.MustCompile(`assets/[0-9a-f]{16}_([^{}\\/\s]+)`)
	for _, match := range assetRef.FindAllStringSubmatch(latexCode, -1) {
		if len(match) < 2 || filepath.Base(match[1]) != match[1] {
			continue
		}
		src := filepath.Join(c.cfg.AssetsTemplateDir, match[1])
		if !fileExists(src) {
			continue
		}
		dst := filepath.Join(workingDir, filepath.FromSlash(match[0]))
		if fileExists(dst) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (c *Converter) resolveAssetPath(ref string) string {
	if ref == "" {
		return ""
	}

	if filepath.IsAbs(ref) {
		if fileExists(ref) {
			return ref
		}
		return ""
	}

	candidates := []string{
		filepath.Join(c.cfg.ERTWikiRoot, ref),
		filepath.Join(c.cfg.AssetsTemplateDir, ref),
		ref,
	}
	for _, cand := range candidates {
		if fileExists(cand) {
			return cand
		}
	}

	exts := []string{".png", ".jpg", ".jpeg", ".pdf", ".svg"}
	for _, cand := range candidates {
		for _, ext := range exts {
			withExt := cand + ext
			if fileExists(withExt) {
				return withExt
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func rewriteAssetReferences(mainTexPath string, mapping map[string]string) error {
	b, err := os.ReadFile(mainTexPath)
	if err != nil {
		return err
	}
	content := string(b)
	for oldRef, newRef := range mapping {
		content = strings.ReplaceAll(content, oldRef, newRef)
	}
	return os.WriteFile(mainTexPath, []byte(content), 0o644)
}

func unzipData(data []byte, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		target := filepath.Join(destination, file.Name)
		if !strings.HasPrefix(target, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive path %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err == nil {
			_, err = io.Copy(dst, src)
			dst.Close()
		}
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func zipDirectory(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = relPath
		hdr.Method = zip.Deflate

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}

		r, err := os.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		_, err = io.Copy(w, r)
		return err
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func filterText(text string) string {
	withoutTag := regexp.MustCompile(`\{\.links-list\}`).ReplaceAllString(text, "")

	lines := strings.Split(withoutTag, "\n")
	result := make([]string, 0, len(lines)+8)
	titleRe := regexp.MustCompile(`^##\s`)
	tableTitleRe := regexp.MustCompile(`^##\s+table\s+\{\.tabset\}`)
	tableRowRe := regexp.MustCompile(`^\|.*\|$`)
	tableSeparatorRe := regexp.MustCompile(`^\|[-:]+.*\|$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		result = append(result, line)

		if titleRe.MatchString(line) && !tableTitleRe.MatchString(line) {
			if i+2 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				nextNext := strings.TrimSpace(lines[i+2])
				if tableRowRe.MatchString(nextLine) && tableSeparatorRe.MatchString(nextNext) {
					result = append(result, "")
				}
			}
		}
	}

	return strings.Join(result, "\n")
}

func removeEmojis(text string) string {
	shortcode := regexp.MustCompile(`:[a-z0-9_+\-]+:`)
	clean := shortcode.ReplaceAllString(text, "")

	clean = strings.Map(func(r rune) rune {
		switch {
		case r == 0xFE0F || r == 0x200D:
			return -1
		case r >= 0x1F300 && r <= 0x1FAFF:
			return -1
		case r >= 0x2600 && r <= 0x27BF:
			return -1
		default:
			return r
		}
	}, clean)

	spaces := regexp.MustCompile(`[ \t]+`)
	clean = spaces.ReplaceAllString(clean, " ")
	trail := regexp.MustCompile(`[ \t]+$`)
	lines := strings.Split(clean, "\n")
	for i := range lines {
		lines[i] = trail.ReplaceAllString(lines[i], "")
	}
	return strings.Join(lines, "\n")
}

// repairLegacyTemplateMacros keeps LaTeX created before the strikeout-template
// correction compilable. The editor deliberately allows a user to edit and
// later compile its LaTeX output, so a deployment cannot otherwise repair a
// document that was converted before the corrected template was installed.
func repairLegacyTemplateMacros(latexCode string) string {
	return strings.NewReplacer(
		`\providecommand{\sout}[1]{\st{##1}}`, `\providecommand{\sout}[1]{\st{#1}}`,
		`\providecommand{\sout}[1]{##1}`, `\providecommand{\sout}[1]{#1}`,
	).Replace(latexCode)
}

func addDraftToDocumentClass(texPath string) error {
	b, err := os.ReadFile(texPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\documentclass`) {
			if strings.Contains(line, "[") {
				lines[i] = strings.Replace(line, "[", "[draft,", 1)
			} else {
				lines[i] = strings.Replace(line, `\documentclass{`, `\documentclass[draft]{`, 1)
			}
			break
		}
	}
	return os.WriteFile(texPath, []byte(strings.Join(lines, "\n")), 0o644)
}

func removeDraftFromDocumentClass(texPath string) error {
	b, err := os.ReadFile(texPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\documentclass`) && strings.Contains(line, "draft") {
			line = strings.ReplaceAll(line, "draft,", "")
			line = strings.ReplaceAll(line, ",draft", "")
			line = strings.ReplaceAll(line, "[draft]", "")
			lines[i] = line
			break
		}
	}
	return os.WriteFile(texPath, []byte(strings.Join(lines, "\n")), 0o644)
}
