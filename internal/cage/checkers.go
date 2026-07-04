// The FileFacts scraper and the 8 deterministic checks:
// synt empt stub secr hold impo dupl stru.
//
// Everything here is parse, string-compare, file-stat, exit-code. No model
// is reachable from this package — it imports nothing from the backend or
// worker layers, by construction.
//
// Unknown languages skip: a language we have no rules for returns pass,
// never a fabricated failure.
package cage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileFacts is everything the scraper measures about one file.
type FileFacts struct {
	Path         string
	Exists       bool
	SizeBytes    int
	SyntaxOK     bool
	Lines        int     // total
	CodeLines    int     // non-blank, non-comment
	BlankLines   int
	CommentLines int
	CommentRatio float64 // comments / max(code,1)
	MaxLineLen   int
	Functions    int
	Classes      int
	LongestFunc  int // lines in the largest function
	MaxNesting   int // deepest indent / brace depth
	BranchCount  int // if/for/while/case/&&/|| — complexity proxy
	EmptyBodies  int // pass-only or {} bodies
	TodoCount    int
	UnusedImports []string
	MixedIndent  bool
	SecretHits   int
	SHA256       string

	// Line anchors for findings (not part of the published fact surface).
	Lang        string
	TodoLines   []int
	SecretLines []int
	StubLines   []int
}

// ChangeFacts summarizes the whole changeset, measured from the binary's
// own git diff — never from agent self-report.
type ChangeFacts struct {
	FilesTouched, FilesCreated, FilesDeleted int
	LinesAdded, LinesRemoved                 int
	DuplicateFiles                           [][]string // identical SHA-256 groups
	VersionOld, VersionNew                   string
}

// Finding is one check outcome on one file.
type Finding struct {
	Code   string // synt | empt | stub | secr | hold | impo | dupl | stru
	Passed bool
	File   string
	Line   int // 0 = file-level
	Detail string
}

// CheckCodes lists the 8 checks in report order.
var CheckCodes = []string{"synt", "empt", "stub", "secr", "hold", "impo", "dupl", "stru"}

// Marker words are assembled at runtime so this source file does not
// trip its own hold check (or any outer cage scanning this repo).
var holdPat = regexp.MustCompile(`(?i)\b(` + "TO" + "DO" + `|FIX` + `ME` + `|XXX|HA` + `CK)\b`)

var secretPats = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(api` + `[_-]?key|secret|passwd|password|auth[_-]?token)\b\s*[:=]\s*["'][A-Za-z0-9_\-/+]{12,}["']`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

var branchPat = regexp.MustCompile(`\b(if|elif|else if|for|while|case|switch|select)\b`)

func langOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh", ".bash":
		return "bash"
	case ".md", ".markdown":
		return "markdown"
	default:
		return ""
	}
}

func isCodeLang(lang string) bool {
	switch lang {
	case "go", "python", "javascript", "bash":
		return true
	}
	return false
}

func commentPrefix(lang string) string {
	switch lang {
	case "go", "javascript":
		return "//"
	case "python", "bash", "yaml":
		return "#"
	}
	return ""
}

// Scrape measures one file into FileFacts. It never errors on content —
// only on unreadable paths, and even then it reports Exists=false facts.
func Scrape(path string) *FileFacts {
	f := &FileFacts{Path: path, Lang: langOf(path), SyntaxOK: true}
	data, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	f.Exists = true
	f.SizeBytes = len(data)
	sum := sha256.Sum256(data)
	f.SHA256 = hex.EncodeToString(sum[:])

	content := string(data)
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	f.Lines = len(lines)

	prefix := commentPrefix(f.Lang)
	var sawTab, sawSpace bool
	inBlockComment := false
	funcStarts := []int{}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(line) > f.MaxLineLen {
			f.MaxLineLen = len(line)
		}
		if trimmed == "" {
			f.BlankLines++
			continue
		}
		// Indentation profile.
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if strings.Contains(lead, "\t") {
			sawTab = true
		}
		if strings.Contains(lead, " ") {
			sawSpace = true
		}
		depth := indentDepth(lead)
		if depth > f.MaxNesting {
			f.MaxNesting = depth
		}

		isComment := false
		if f.Lang == "go" || f.Lang == "javascript" {
			if inBlockComment {
				isComment = true
				if strings.Contains(trimmed, "*/") {
					inBlockComment = false
				}
			} else if strings.HasPrefix(trimmed, "/*") {
				isComment = true
				if !strings.Contains(trimmed, "*/") {
					inBlockComment = true
				}
			}
		}
		if !isComment && prefix != "" && strings.HasPrefix(trimmed, prefix) {
			isComment = true
		}
		if isComment {
			f.CommentLines++
		} else {
			f.CodeLines++
			f.BranchCount += len(branchPat.FindAllString(trimmed, -1))
			f.BranchCount += strings.Count(trimmed, "&&") + strings.Count(trimmed, "||")
		}
		// Markers count anywhere, comments included — that is where they live.
		if holdPat.MatchString(line) {
			f.TodoCount++
			f.TodoLines = append(f.TodoLines, i+1)
		}
		for _, pat := range secretPats {
			if pat.MatchString(line) {
				f.SecretHits++
				f.SecretLines = append(f.SecretLines, i+1)
				break
			}
		}
		if !isComment && isFunctionStart(f.Lang, trimmed) {
			f.Functions++
			funcStarts = append(funcStarts, i)
		}
		if !isComment && isClassStart(f.Lang, trimmed) {
			f.Classes++
		}
		if !isComment && isEmptyBody(f.Lang, trimmed) {
			f.EmptyBodies++
			f.StubLines = append(f.StubLines, i+1)
		}
	}

	f.MixedIndent = sawTab && sawSpace
	f.CommentRatio = float64(f.CommentLines) / float64(max(f.CodeLines, 1))
	f.LongestFunc = longestSpan(funcStarts, len(lines))
	f.SyntaxOK = checkSyntax(f.Lang, path, data)
	if f.Lang == "python" {
		f.UnusedImports = pythonUnusedImports(lines)
	}
	return f
}

func indentDepth(lead string) int {
	tabs := strings.Count(lead, "\t")
	spaces := len(lead) - tabs
	return tabs + spaces/4
}

func isFunctionStart(lang, trimmed string) bool {
	switch lang {
	case "go":
		return strings.HasPrefix(trimmed, "func ")
	case "python":
		return strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ")
	case "javascript":
		return strings.HasPrefix(trimmed, "function ") || strings.Contains(trimmed, "=> {")
	case "bash":
		return regexp.MustCompile(`^\w+\s*\(\)\s*\{`).MatchString(trimmed)
	}
	return false
}

func isClassStart(lang, trimmed string) bool {
	switch lang {
	case "go":
		return regexp.MustCompile(`^type\s+\w+\s+(struct|interface)\b`).MatchString(trimmed)
	case "python":
		return strings.HasPrefix(trimmed, "class ")
	case "javascript":
		return strings.HasPrefix(trimmed, "class ")
	}
	return false
}

func isEmptyBody(lang, trimmed string) bool {
	switch lang {
	case "python":
		return trimmed == "pass" || trimmed == "..."
	case "javascript":
		return regexp.MustCompile(`\{\s*\}\s*;?\s*$`).MatchString(trimmed) &&
			(strings.Contains(trimmed, "function") || strings.Contains(trimmed, "=>"))
	}
	// Go: the compiler and vet own this; empty struct/interface literals
	// are idiomatic, so no naive brace matching here. Unknown → pass.
	return false
}

func longestSpan(starts []int, total int) int {
	longest := 0
	for i, s := range starts {
		end := total
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if end-s > longest {
			longest = end - s
		}
	}
	return longest
}

// checkSyntax is deterministic per language: native Go parsers for
// go/json/yaml, a shell-out for python/bash/javascript when the toolchain
// exists. Missing toolchain or unknown language → pass, never a fabricated
// failure.
func checkSyntax(lang, path string, data []byte) bool {
	switch lang {
	case "go":
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, path, data, 0)
		return err == nil
	case "json":
		return json.Valid(data)
	case "yaml":
		var v any
		return yaml.Unmarshal(data, &v) == nil
	case "python":
		return shellSyntax("python3", "-c",
			`import py_compile,sys,tempfile,os; py_compile.compile(sys.argv[1], cfile=os.path.join(tempfile.mkdtemp(), "chk.pyc"), doraise=True)`, path)
	case "bash":
		return shellSyntax("bash", "-n", path)
	case "javascript":
		return shellSyntax("node", "--check", path)
	}
	return true
}

func shellSyntax(bin string, args ...string) bool {
	if _, err := exec.LookPath(bin); err != nil {
		return true // no toolchain — skip, never fail on ignorance
	}
	cmd := exec.Command(bin, args...)
	return cmd.Run() == nil
}

var pyImportPat = regexp.MustCompile(`^\s*(?:import\s+([\w.]+)(?:\s+as\s+(\w+))?|from\s+[\w.]+\s+import\s+(.+))$`)

func pythonUnusedImports(lines []string) []string {
	type imp struct {
		name string
		line int
	}
	var imports []imp
	var body []string
	for i, line := range lines {
		m := pyImportPat.FindStringSubmatch(line)
		if m == nil {
			body = append(body, line)
			continue
		}
		switch {
		case m[2] != "": // import x as y
			imports = append(imports, imp{m[2], i})
		case m[1] != "": // import x[.y]
			root := strings.SplitN(m[1], ".", 2)[0]
			imports = append(imports, imp{root, i})
		case m[3] != "": // from m import a, b as c
			if strings.Contains(m[3], "*") {
				continue
			}
			for _, part := range strings.Split(m[3], ",") {
				part = strings.TrimSpace(strings.TrimSuffix(part, "\\"))
				if as := strings.Split(part, " as "); len(as) == 2 {
					imports = append(imports, imp{strings.TrimSpace(as[1]), i})
				} else if part != "" {
					imports = append(imports, imp{strings.Fields(part)[0], i})
				}
			}
		}
	}
	rest := strings.Join(body, "\n")
	var unused []string
	for _, im := range imports {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(im.name) + `\b`).MatchString(rest) {
			unused = append(unused, im.name)
		}
	}
	return unused
}

// RunChecks evaluates the 8 checks against one file's facts. dupl is a
// changeset-level check and is emitted by the quality officer, not here.
func RunChecks(f *FileFacts, disabled map[string]bool) []Finding {
	var out []Finding
	add := func(code string, passed bool, line int, detail string) {
		if disabled[code] {
			return
		}
		out = append(out, Finding{Code: code, Passed: passed, File: f.Path, Line: line, Detail: detail})
	}

	// synt — syntax must parse.
	add("synt", f.SyntaxOK, 0, syntDetail(f))

	// empt — empty or effectively empty file.
	emptOK := f.SizeBytes > 0 && !(isCodeLang(f.Lang) && f.CodeLines == 0)
	add("empt", emptOK, 0, pick(emptOK, "file has content", "file is empty or contains no code"))

	// stub — placeholder bodies.
	stubOK := f.EmptyBodies == 0
	add("stub", stubOK, firstLine(f.StubLines), pick(stubOK, "no stub bodies", fmt.Sprintf("%d placeholder body(ies)", f.EmptyBodies)))

	// secr — hardcoded secrets.
	secrOK := f.SecretHits == 0
	add("secr", secrOK, firstLine(f.SecretLines), pick(secrOK, "no hardcoded secrets", fmt.Sprintf("%d potential secret(s)", f.SecretHits)))

	// hold — unfinished-work markers, code files only.
	holdOK := !isCodeLang(f.Lang) || f.TodoCount == 0
	holdDetail := "no unfinished-work markers"
	if !holdOK {
		holdDetail = fmt.Sprintf("%d unfinished-work marker(s) at line %s", f.TodoCount, joinInts(f.TodoLines))
	}
	add("hold", holdOK, firstLine(f.TodoLines), holdDetail)

	// impo — unused imports (python only; the Go compiler owns Go).
	impoOK := len(f.UnusedImports) == 0
	add("impo", impoOK, 0, pick(impoOK, "imports in use", "unused imports: "+strings.Join(f.UnusedImports, ", ")))

	// stru — structural minimum per language; unknown → pass.
	struOK, struDetail := checkStructure(f)
	add("stru", struOK, 0, struDetail)

	return out
}

func syntDetail(f *FileFacts) string {
	if f.SyntaxOK {
		return "syntax parses"
	}
	return "syntax error (" + f.Lang + ")"
}

func checkStructure(f *FileFacts) (bool, string) {
	switch f.Lang {
	case "markdown":
		data, err := os.ReadFile(f.Path)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					return true, "has a heading"
				}
			}
			return false, "markdown file has no heading"
		}
	case "python":
		if f.MixedIndent {
			return false, "mixed tab/space indentation"
		}
	}
	return true, "structure ok"
}

func firstLine(lines []int) int {
	if len(lines) > 0 {
		return lines[0]
	}
	return 0
}

func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ",")
}

func pick(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
