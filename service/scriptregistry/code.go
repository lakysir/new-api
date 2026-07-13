// Package scriptregistry implements the trusted market-script supply chain:
// code normalization, SHA-256 content addressing, static safety scanning and
// Ed25519 manifest signing. It is intentionally free of HTTP/GORM concerns so
// the security-critical rules can be unit tested in isolation.
package scriptregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// MaxCodeBytes bounds published script code. Keep in sync with the draft limit
// in model.UserScriptMaxCodeLength.
const MaxCodeBytes = 1024 * 1024

// RequiredEntry is the single execution entry every market script must expose.
const RequiredEntry = "runGeneratedTest"

var (
	// ErrEmptyCode is returned when there is nothing to publish.
	ErrEmptyCode = errors.New("script code is empty")
	// ErrCodeTooLarge is returned when normalized code exceeds MaxCodeBytes.
	ErrCodeTooLarge = errors.New("script code is too large")
	// ErrMissingEntry is returned when runGeneratedTest(config) is absent.
	ErrMissingEntry = errors.New("script must expose async function runGeneratedTest(config)")
)

// NormalizeCode canonicalizes code before hashing so that a byte-identical
// publish always produces the same SHA-256: it normalizes line endings and
// trims a trailing newline run, without touching interior bytes.
func NormalizeCode(code string) string {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	code = strings.ReplaceAll(code, "\r", "\n")
	code = strings.TrimRight(code, "\n") + "\n"
	return code
}

// CodeSha256 returns the "sha256:<hex>" content address of the given bytes.
func CodeSha256(code string) string {
	sum := sha256.Sum256([]byte(code))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// entryPattern matches an exported runGeneratedTest function declaration in the
// forms the analyzer generates: `async function runGeneratedTest(config)` or a
// `runGeneratedTest = async (config) =>` assignment.
var entryPattern = regexp.MustCompile(`(?m)\brunGeneratedTest\s*(=\s*(async\s*)?(function)?\s*)?\(`)

// DangerFinding is a single static-scan hit against the code.
type DangerFinding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// dangerRule pairs a compiled matcher with a human-readable reason.
type dangerRule struct {
	rule    string
	message string
	re      *regexp.Regexp
}

// dangerRules flags the high-risk primitives called out in architecture §7.1.
// The scan is advisory input to review, not a sandbox: MAIN-world scripts share
// the page's authority, so signing + origin allow-listing remain the real
// boundary. Matches are conservative (may over-flag) to force human review.
var dangerRules = []dangerRule{
	{"eval", "use of eval()", regexp.MustCompile(`\beval\s*\(`)},
	{"function_constructor", "dynamic Function() constructor", regexp.MustCompile(`\bnew\s+Function\s*\(`)},
	{"dynamic_import", "dynamic import()", regexp.MustCompile(`\bimport\s*\(`)},
	{"import_scripts", "importScripts()", regexp.MustCompile(`\bimportScripts\s*\(`)},
	{"remote_script_tag", "injecting a remote <script> src", regexp.MustCompile(`\.src\s*=\s*['"]?https?:`)},
	{"chrome_api", "probing chrome.* extension APIs", regexp.MustCompile(`\bchrome\.\w+`)},
	{"document_cookie", "accessing document.cookie", regexp.MustCompile(`document\.cookie`)},
	{"local_storage", "accessing localStorage/sessionStorage", regexp.MustCompile(`\b(localStorage|sessionStorage)\b`)},
	{"indexeddb", "accessing indexedDB", regexp.MustCompile(`\bindexedDB\b`)},
	{"set_timeout_string", "setTimeout/setInterval with a string body", regexp.MustCompile(`\b(setTimeout|setInterval)\s*\(\s*['"]`)},
}

// ScanCode runs the static safety scan and returns all findings. An empty slice
// means no known-dangerous primitive was detected.
func ScanCode(code string) []DangerFinding {
	findings := make([]DangerFinding, 0)
	for _, r := range dangerRules {
		if r.re.MatchString(code) {
			findings = append(findings, DangerFinding{Rule: r.rule, Message: r.message})
		}
	}
	return findings
}

// HasEntry reports whether the code exposes a runGeneratedTest(config) entry.
func HasEntry(code string) bool {
	return entryPattern.MatchString(code)
}

// ValidatePublishable enforces the pre-publish invariants and returns the
// normalized code plus its content hash. Static findings are returned
// separately so the caller (review flow) can decide to require human sign-off
// rather than hard-fail; only structural problems error here.
func ValidatePublishable(code string) (normalized string, codeSha256 string, findings []DangerFinding, err error) {
	if strings.TrimSpace(code) == "" {
		return "", "", nil, ErrEmptyCode
	}
	normalized = NormalizeCode(code)
	if len(normalized) > MaxCodeBytes {
		return "", "", nil, ErrCodeTooLarge
	}
	if !HasEntry(normalized) {
		return "", "", nil, ErrMissingEntry
	}
	return normalized, CodeSha256(normalized), ScanCode(normalized), nil
}
