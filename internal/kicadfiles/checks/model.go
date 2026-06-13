package checks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	defaultTimeout = 60 * time.Second
)

type CheckKind string

const (
	CheckKindERC CheckKind = "erc"
	CheckKindDRC CheckKind = "drc"
)

type CheckStatus string

const (
	CheckStatusPass    CheckStatus = "pass"
	CheckStatusFail    CheckStatus = "fail"
	CheckStatusSkipped CheckStatus = "skipped"
	CheckStatusError   CheckStatus = "error"
)

type FileType string

const (
	FileTypeSchematic FileType = "schematic"
	FileTypePCB       FileType = "pcb"
	FileTypeProject   FileType = "project"
)

type ProjectContext string

const (
	ProjectContextFull       ProjectContext = "full"
	ProjectContextStandalone ProjectContext = "standalone"
	ProjectContextUnknown    ProjectContext = "unknown"
)

type RepairCategory string

const (
	RepairConnectivity  RepairCategory = "connectivity"
	RepairClearance     RepairCategory = "clearance"
	RepairOutline       RepairCategory = "outline"
	RepairFootprint     RepairCategory = "footprint"
	RepairNetAssignment RepairCategory = "net_assignment"
	RepairPower         RepairCategory = "power"
	RepairNoConnect     RepairCategory = "no_connect"
	RepairMetadata      RepairCategory = "metadata"
	RepairUnknown       RepairCategory = "unknown"
)

type Options struct {
	KiCadCLI      string
	KeepArtifacts bool
	ArtifactDir   string
	Timeout       time.Duration
	Allowlist     []AllowlistEntry
	Units         string
}

func (opts Options) withDefaults() Options {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if strings.TrimSpace(opts.Units) == "" {
		opts.Units = "mm"
	}
	return opts
}

func DefaultOptions() Options {
	return Options{}.withDefaults()
}

type CommandResult struct {
	Path       string        `json:"path,omitempty"`
	Args       []string      `json:"args,omitempty"`
	WorkingDir string        `json:"working_dir,omitempty"`
	Stdout     string        `json:"stdout,omitempty"`
	Stderr     string        `json:"stderr,omitempty"`
	ExitCode   int           `json:"exit_code,omitempty"`
	Duration   time.Duration `json:"-"`
	Err        error         `json:"-"`
}

type CheckResult struct {
	Kind            CheckKind        `json:"kind"`
	Status          CheckStatus      `json:"status"`
	TargetPath      string           `json:"target_path"`
	FileType        FileType         `json:"file_type"`
	ProjectContext  ProjectContext   `json:"project_context,omitempty"`
	Units           string           `json:"units,omitempty"`
	KiCadCLIPath    string           `json:"kicad_cli_path,omitempty"`
	KiCadVersion    string           `json:"kicad_version,omitempty"`
	Command         []string         `json:"command,omitempty"`
	WorkingDir      string           `json:"working_dir,omitempty"`
	ExitCode        int              `json:"exit_code,omitempty"`
	DurationMS      int64            `json:"duration_ms,omitempty"`
	ReportPath      string           `json:"report_path,omitempty"`
	Stdout          string           `json:"stdout,omitempty"`
	Stderr          string           `json:"stderr,omitempty"`
	Findings        []CheckFinding   `json:"findings,omitempty"`
	Allowed         []CheckFinding   `json:"allowed,omitempty"`
	ParserIssues    []ParserIssue    `json:"parser_issues,omitempty"`
	ContextWarnings []ContextWarning `json:"context_warnings,omitempty"`
}

type CheckFinding struct {
	ID             string         `json:"id,omitempty"`
	Kind           CheckKind      `json:"kind"`
	Severity       string         `json:"severity"`
	Rule           string         `json:"rule,omitempty"`
	Code           string         `json:"code,omitempty"`
	Message        string         `json:"message"`
	File           string         `json:"file,omitempty"`
	Sheet          string         `json:"sheet,omitempty"`
	References     []string       `json:"references,omitempty"`
	Pins           []string       `json:"pins,omitempty"`
	Net            string         `json:"net,omitempty"`
	Nets           []string       `json:"nets,omitempty"`
	Layer          string         `json:"layer,omitempty"`
	Location       *CheckLocation `json:"location,omitempty"`
	Objects        []CheckObject  `json:"objects,omitempty"`
	Raw            string         `json:"raw,omitempty"`
	RepairCategory RepairCategory `json:"repair_category,omitempty"`
}

type CheckLocation struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Units string  `json:"units,omitempty"`
}

type CheckObject struct {
	Type      string `json:"type,omitempty"`
	ID        string `json:"id,omitempty"`
	Reference string `json:"reference,omitempty"`
	Pad       string `json:"pad,omitempty"`
	Net       string `json:"net,omitempty"`
	Layer     string `json:"layer,omitempty"`
}

type ParserIssue struct {
	Message string `json:"message"`
	Raw     string `json:"raw,omitempty"`
}

type ContextWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

func ClassifyStatus(toolError bool, skipped bool, findings []CheckFinding, parserIssues []ParserIssue) CheckStatus {
	switch {
	case skipped:
		return CheckStatusSkipped
	case toolError || len(parserIssues) > 0:
		return CheckStatusError
	case len(findings) > 0:
		return CheckStatusFail
	default:
		return CheckStatusPass
	}
}

func NormalizeFindings(kind CheckKind, findings []CheckFinding) []CheckFinding {
	items := make([]normalizedFinding, len(findings))
	for i, finding := range findings {
		finding.Kind = kind
		if finding.RepairCategory == "" {
			finding.RepairCategory = ClassifyRepairCategory(finding)
		}
		finding.References = sortedStrings(finding.References)
		finding.Pins = sortedStrings(finding.Pins)
		finding.Nets = sortedStrings(finding.Nets)
		if finding.ID == "" {
			finding.ID = StableFindingID(finding)
		}
		items[i] = normalizedFinding{finding: finding, sortKey: newFindingSortKey(finding)}
	}
	slices.SortFunc(items, func(a, b normalizedFinding) int {
		return strings.Compare(a.sortKey, b.sortKey)
	})
	out := make([]CheckFinding, len(items))
	for i, item := range items {
		out[i] = item.finding
	}
	return out
}

type normalizedFinding struct {
	finding CheckFinding
	sortKey string
}

func StableFindingID(f CheckFinding) string {
	parts := []string{
		string(f.Kind),
		normalizeKey(f.Severity),
		normalizeKey(firstNonEmpty(f.Rule, f.Code)),
		normalizeKey(f.File),
		normalizeKey(f.Sheet),
		strings.Join(sortedStrings(f.References), ","),
		strings.Join(sortedStrings(f.Pins), ","),
		normalizeKey(f.Net),
		strings.Join(sortedStrings(f.Nets), ","),
		normalizeKey(f.Layer),
		normalizeLocation(f.Location),
		normalizeObjects(f.Objects),
		normalizeKey(f.Message),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:32]
}

func ClassifyRepairCategory(f CheckFinding) RepairCategory {
	text := normalizeKey(strings.Join([]string{f.Rule, f.Code, f.Message}, " "))
	switch {
	case containsAny(text, "clearance", "courtyard", "silk", "solder mask"):
		return RepairClearance
	case containsAny(text, "edge cuts", "outline", "board edge", "board has malformed outline"):
		return RepairOutline
	case containsAny(text, "footprint", "pad", "drill"):
		return RepairFootprint
	case containsAny(text, "net", "parity", "unconnected", "not connected", "short"):
		return RepairConnectivity
	case containsAny(text, "power"):
		return RepairPower
	case containsAny(text, "no connect", "no-connect", "noconnect"):
		return RepairNoConnect
	case containsAny(text, "library", "variable", "field", "metadata"):
		return RepairMetadata
	default:
		return RepairUnknown
	}
}

func newFindingSortKey(f CheckFinding) string {
	return strings.Join([]string{
		severityRank(f.Severity),
		normalizeKey(firstNonEmpty(f.Rule, f.Code)),
		normalizeKey(f.File),
		normalizeKey(f.Sheet),
		strings.Join(f.References, ","),
		normalizeKey(f.Net),
		normalizeKey(f.Layer),
		normalizeLocation(f.Location),
		normalizeKey(f.Message),
		f.ID,
	}, "\x00")
}

func severityRank(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error", "fatal":
		return "0"
	case "warning", "warn":
		return "1"
	case "exclusion", "excluded":
		return "2"
	default:
		return "3"
	}
}

func normalizeLocation(loc *CheckLocation) string {
	if loc == nil {
		return ""
	}
	return fmt.Sprintf("%.6f,%.6f,%s", loc.X, loc.Y, normalizeKey(loc.Units))
}

func normalizeObjects(objects []CheckObject) string {
	if len(objects) == 0 {
		return ""
	}
	values := make([]string, 0, len(objects))
	for _, object := range objects {
		values = append(values, strings.Join([]string{
			normalizeKey(object.Type),
			normalizeKey(object.Reference),
			normalizeKey(object.Pad),
			normalizeKey(object.Net),
			normalizeKey(object.Layer),
			normalizeKey(object.ID),
		}, ":"))
	}
	slices.Sort(values)
	return strings.Join(values, ",")
}

func normalizeKey(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sortedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	slices.Sort(out)
	return out
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
