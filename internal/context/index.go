package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

// IndexDocument is the lightweight project context index stored under Atlas Home.
type IndexDocument struct {
	SchemaVersion int         `yaml:"schema_version"`
	ProjectRoot   string      `yaml:"project_root"`
	ProjectID     string      `yaml:"project_id"`
	ProjectName   string      `yaml:"project_name,omitempty"`
	IndexedAt     string      `yaml:"indexed_at"`
	AtlasVersion  string      `yaml:"atlas_version,omitempty"`
	Fingerprint   string      `yaml:"fingerprint"`
	Languages     []string    `yaml:"languages,omitempty"`
	Frameworks    []string    `yaml:"frameworks,omitempty"`
	Directories   []string    `yaml:"directories,omitempty"`
	Files         []IndexFile `yaml:"files,omitempty"`
	RuntimeAtlas  []string    `yaml:"runtime_atlas,omitempty"`
	ConfigFiles   []string    `yaml:"config_files,omitempty"`
	TestFiles     []string    `yaml:"test_files,omitempty"`
	Docs          []string    `yaml:"docs,omitempty"`
	Manifests     []string    `yaml:"manifests,omitempty"`
	Contracts     []string    `yaml:"contracts,omitempty"`
	Entrypoints   []string    `yaml:"entrypoints,omitempty"`
	IgnoredDirs   int         `yaml:"ignored_dirs"`
	IgnoredFiles  int         `yaml:"ignored_files"`
	Truncated     bool        `yaml:"truncated,omitempty"`
	Notes         []string    `yaml:"notes,omitempty"`
}

// IndexFile is one indexed file entry.
type IndexFile struct {
	Path string `yaml:"path"`
	Kind string `yaml:"kind"`
	Size int64  `yaml:"size"`
}

const indexSchemaVersion = 1

// BuildIndex walks the project root and builds a deterministic index. Read-only.
func BuildIndex(root string, projectName string, now time.Time) (IndexDocument, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return IndexDocument{}, err
	}
	abs = filepath.Clean(abs)
	id, err := ProjectID(abs, projectName)
	if err != nil {
		return IndexDocument{}, err
	}
	ver := version.Version
	if ver == "" {
		ver = "0.1.0"
	}

	idx := IndexDocument{
		SchemaVersion: indexSchemaVersion,
		ProjectRoot:   abs,
		ProjectID:     id,
		ProjectName:   strings.TrimSpace(projectName),
		IndexedAt:     now.UTC().Format(time.RFC3339),
		AtlasVersion:  ver,
		Files:         []IndexFile{},
		Directories:   []string{},
		RuntimeAtlas:  []string{},
		ConfigFiles:   []string{},
		TestFiles:     []string{},
		Docs:          []string{},
		Manifests:     []string{},
		Contracts:     []string{},
		Entrypoints:   []string{},
		Languages:     []string{},
		Frameworks:    []string{},
		Notes:         []string{},
	}

	var dirs []string
	var files []IndexFile
	fp := sha256.New()

	err = filepath.Walk(abs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if ShouldIgnoreDir(info.Name()) || ShouldIgnoreRel(rel) {
				idx.IgnoredDirs++
				return filepath.SkipDir
			}
			dirs = append(dirs, rel)
			if !skipFingerprint(rel) {
				fmt.Fprintf(fp, "d\x00%s\n", rel)
			}
			return nil
		}
		if ShouldIgnoreRel(rel) {
			idx.IgnoredFiles++
			return nil
		}
		if info.Size() > MaxFileBytes {
			idx.IgnoredFiles++
			return nil
		}
		if len(files) >= MaxIndexedEntries {
			idx.Truncated = true
			return nil
		}
		kind := classifyPath(rel)
		entry := IndexFile{Path: rel, Kind: kind, Size: info.Size()}
		files = append(files, entry)
		// Exclude self-mutating Atlas metadata from freshness fingerprint.
		if !skipFingerprint(rel) {
			fmt.Fprintf(fp, "f\x00%s\x00%d\x00%d\n", rel, info.Size(), info.ModTime().UTC().Unix())
		}
		return nil
	})
	if err != nil {
		return IndexDocument{}, fmt.Errorf("context index: walk: %w", err)
	}

	sort.Strings(dirs)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	idx.Directories = dirs
	idx.Files = files
	idx.Fingerprint = hex.EncodeToString(fp.Sum(nil))

	for _, f := range files {
		switch f.Kind {
		case "runtime_atlas":
			idx.RuntimeAtlas = append(idx.RuntimeAtlas, f.Path)
		case "config":
			idx.ConfigFiles = append(idx.ConfigFiles, f.Path)
		case "test":
			idx.TestFiles = append(idx.TestFiles, f.Path)
		case "doc":
			idx.Docs = append(idx.Docs, f.Path)
		case "manifest":
			idx.Manifests = append(idx.Manifests, f.Path)
		case "contract":
			idx.Contracts = append(idx.Contracts, f.Path)
		case "entrypoint":
			idx.Entrypoints = append(idx.Entrypoints, f.Path)
		}
	}
	idx.Languages, idx.Frameworks = detectLanguages(files)
	if idx.Truncated {
		idx.Notes = append(idx.Notes, fmt.Sprintf("index truncated at %d files", MaxIndexedEntries))
	}
	return idx, nil
}

// LoadIndex loads an index.yaml from disk. Read-only.
func LoadIndex(path string) (IndexDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return IndexDocument{}, err
	}
	var idx IndexDocument
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return IndexDocument{}, fmt.Errorf("context index: parse %s: %w", path, err)
	}
	return idx, nil
}

// RenderIndexYAML marshals an index deterministically.
func RenderIndexYAML(idx IndexDocument) (string, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&idx); err != nil {
		_ = enc.Close()
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func skipFingerprint(rel string) bool {
	// Atlas-owned metadata churn (including Context Economy state refs) must not
	// mark the product tree fingerprint stale. Runtime drift is tracked separately.
	clean := filepath.ToSlash(rel)
	return clean == ".atlas" || strings.HasPrefix(clean, ".atlas/")
}

func classifyPath(rel string) string {
	clean := filepath.ToSlash(rel)
	base := filepath.Base(clean)
	lower := strings.ToLower(clean)

	switch {
	case clean == config.FileAgentsMD,
		clean == config.FileCursorAtlasMDC,
		clean == config.FileOpenCodeAtlas,
		strings.HasPrefix(clean, config.DirCursorAgents+"/"),
		strings.HasPrefix(clean, config.DirOpenCodeAgents+"/"),
		clean == config.FileAgentRegistry,
		clean == config.FileRuntimeManifest,
		clean == config.FileAssetsLock,
		clean == config.FileConfig,
		clean == config.FileState:
		return "runtime_atlas"
	case clean == config.FileSDDOpenSpecContract || strings.HasPrefix(clean, ".atlas/contracts/"):
		return "contract"
	case strings.HasPrefix(clean, ".atlas/"):
		return "config"
	case base == "go.mod", base == "go.sum", base == "package.json", base == "package-lock.json",
		base == "pnpm-lock.yaml", base == "yarn.lock", base == "Cargo.toml", base == "Cargo.lock",
		base == "composer.json", base == "composer.lock", base == "pyproject.toml",
		base == "requirements.txt", base == "Gemfile", base == "Podfile",
		base == "Makefile", base == "makefile", base == "Dockerfile",
		strings.HasPrefix(base, "docker-compose"):
		return "manifest"
	case strings.HasSuffix(lower, "_test.go"), strings.HasSuffix(lower, "_test.py"),
		strings.HasSuffix(lower, ".test.ts"), strings.HasSuffix(lower, ".test.js"),
		strings.HasSuffix(lower, ".spec.ts"), strings.HasSuffix(lower, ".spec.js"),
		strings.Contains(lower, "/testdata/"), strings.HasPrefix(lower, "test/"),
		strings.HasPrefix(lower, "tests/"), strings.Contains(lower, "/__tests__/"):
		return "test"
	case strings.HasSuffix(lower, ".md"), strings.HasPrefix(lower, "docs/"),
		base == "readme", strings.HasPrefix(base, "readme."):
		if clean == config.FileAgentsMD {
			return "runtime_atlas"
		}
		return "doc"
	case base == "main.go", base == "main.ts", base == "main.js", base == "index.ts",
		base == "index.js", base == "app.go", strings.HasPrefix(clean, "cmd/"),
		strings.HasPrefix(clean, "src/main/"):
		return "entrypoint"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
		strings.HasSuffix(lower, ".toml"), strings.HasSuffix(lower, ".json"),
		strings.HasSuffix(lower, ".env.example"), base == ".gitignore", base == ".editorconfig":
		return "config"
	default:
		return "source"
	}
}

func detectLanguages(files []IndexFile) (languages, frameworks []string) {
	seenLang := map[string]bool{}
	seenFW := map[string]bool{}
	addLang := func(name string) {
		if !seenLang[name] {
			seenLang[name] = true
			languages = append(languages, name)
		}
	}
	addFW := func(name string) {
		if !seenFW[name] {
			seenFW[name] = true
			frameworks = append(frameworks, name)
		}
	}
	for _, f := range files {
		base := filepath.Base(f.Path)
		ext := strings.ToLower(filepath.Ext(f.Path))
		switch {
		case base == "go.mod" || ext == ".go":
			addLang("Go")
		case base == "package.json" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx":
			addLang("JavaScript/TypeScript")
		case base == "Cargo.toml" || ext == ".rs":
			addLang("Rust")
		case base == "pyproject.toml" || ext == ".py":
			addLang("Python")
		case base == "composer.json" || ext == ".php":
			addLang("PHP")
		case ext == ".java" || base == "pom.xml" || base == "build.gradle":
			addLang("Java")
		}
		switch base {
		case "go.mod":
			addFW("Go modules")
		case "package.json":
			addFW("Node.js")
		case "docker-compose.yml", "docker-compose.yaml":
			addFW("Docker Compose")
		case "Dockerfile":
			addFW("Docker")
		}
	}
	sort.Strings(languages)
	sort.Strings(frameworks)
	return languages, frameworks
}
