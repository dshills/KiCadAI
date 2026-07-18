package fabrication

import (
	"cmp"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/fabrication/physicalrules"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

const ManifestSchema = "kicadai.fabrication.package.v1"

type ReadinessStatus string

const (
	StatusBlocked   ReadinessStatus = "blocked"
	StatusCandidate ReadinessStatus = "candidate"
	StatusReady     ReadinessStatus = "ready"
)

type ArtifactKind string

const (
	ArtifactBOM             ArtifactKind = "bom"
	ArtifactCPL             ArtifactKind = "cpl"
	ArtifactManifest        ArtifactKind = "manifest"
	ArtifactGerber          ArtifactKind = "gerber"
	ArtifactDrill           ArtifactKind = "drill"
	ArtifactERC             ArtifactKind = "erc"
	ArtifactDRC             ArtifactKind = "drc"
	ArtifactBlockReadiness  ArtifactKind = "block_readiness"
	ArtifactPhysicalRules   ArtifactKind = "physical_rules"
	ArtifactReadinessReport ArtifactKind = "readiness_report"
)

type ArtifactStatus string

const (
	ArtifactExpected  ArtifactStatus = "expected"
	ArtifactGenerated ArtifactStatus = "generated"
	ArtifactMissing   ArtifactStatus = "missing"
	ArtifactSkipped   ArtifactStatus = "skipped"
	ArtifactBlocked   ArtifactStatus = "blocked"
)

type EvidenceStatus string

const (
	EvidencePass    EvidenceStatus = "pass"
	EvidenceWarning EvidenceStatus = "warning"
	EvidenceMissing EvidenceStatus = "missing"
	EvidenceSkipped EvidenceStatus = "skipped"
	EvidenceFail    EvidenceStatus = "fail"
)

type Generator string

const (
	GeneratorKiCadAI  Generator = "kicadai"
	GeneratorKiCad    Generator = "kicad-cli"
	GeneratorExternal Generator = "external"
)

type CLIPolicy string

const (
	CLIPolicyDisabled CLIPolicy = "disabled"
	CLIPolicyOptional CLIPolicy = "optional"
	CLIPolicyRequired CLIPolicy = "required"
)

type ProjectRef struct {
	Name string `json:"name"`
	Root string `json:"root,omitempty"`
}

type Artifact struct {
	Kind        ArtifactKind    `json:"kind"`
	Path        string          `json:"path"`
	Status      ArtifactStatus  `json:"status"`
	Required    bool            `json:"required,omitempty"`
	Generator   Generator       `json:"generator,omitempty"`
	Description string          `json:"description,omitempty"`
	Files       []string        `json:"files,omitempty"`
	Issues      []reports.Issue `json:"issues,omitempty"`
}

type Summary struct {
	Generated           bool           `json:"generated"`
	Project             EvidenceStatus `json:"project"`
	Schematic           EvidenceStatus `json:"schematic"`
	PCB                 EvidenceStatus `json:"pcb"`
	WriterCorrectness   EvidenceStatus `json:"writer_correctness"`
	BoardValidation     EvidenceStatus `json:"board_validation"`
	ERC                 EvidenceStatus `json:"erc"`
	DRC                 EvidenceStatus `json:"drc"`
	BOM                 EvidenceStatus `json:"bom"`
	CPL                 EvidenceStatus `json:"cpl"`
	Gerber              EvidenceStatus `json:"gerber"`
	Drill               EvidenceStatus `json:"drill"`
	Manifest            EvidenceStatus `json:"manifest"`
	ComponentReadiness  EvidenceStatus `json:"component_readiness"`
	BlockReadiness      EvidenceStatus `json:"block_readiness"`
	ComponentIdentity   EvidenceStatus `json:"component_identity"`
	BOMCPLConsistency   EvidenceStatus `json:"bom_cpl_consistency"`
	ManufacturerProfile EvidenceStatus `json:"manufacturer_profile"`
	AssemblyReadiness   EvidenceStatus `json:"assembly_readiness"`
	PhysicalRules       EvidenceStatus `json:"physical_rules"`
}

type Options struct {
	Command                string                       `json:"command,omitempty"`
	Execute                bool                         `json:"execute"`
	Overwrite              bool                         `json:"overwrite"`
	Output                 string                       `json:"output,omitempty"`
	KiCadCLI               string                       `json:"kicad_cli,omitempty"`
	CLIPolicy              CLIPolicy                    `json:"cli_policy,omitempty"`
	ManufacturerProfile    string                       `json:"manufacturer_profile,omitempty"`
	ManufacturerProfileDir string                       `json:"manufacturer_profile_dir,omitempty"`
	SourceDir              string                       `json:"source_dir,omitempty"`
	PlotRunner             PlotRunner                   `json:"-"`
	CheckRunner            checks.Runner                `json:"-"`
	LibraryIndex           libraryresolver.LibraryIndex `json:"-"`
	HasLibraryIndex        bool                         `json:"-"`
	LibraryIssues          []reports.Issue              `json:"-"`
	Sources                *components.SourceCollection `json:"-"`
	BlockReadinessReport   []byte                       `json:"-"`
}

type Manifest struct {
	Schema              string                     `json:"schema"`
	Project             ProjectRef                 `json:"project"`
	Status              ReadinessStatus            `json:"status"`
	Score               int                        `json:"score"`
	Generated           bool                       `json:"generated"`
	CreatedBy           Generator                  `json:"created_by"`
	ManufacturerProfile *physicalrules.ProfileInfo `json:"manufacturer_profile,omitempty"`
	Artifacts           []Artifact                 `json:"artifacts"`
	Evidence            map[string]EvidenceStatus  `json:"evidence"`
	Issues              []reports.Issue            `json:"issues"`
	Options             Options                    `json:"options,omitempty"`
}

type Result struct {
	Status              ReadinessStatus            `json:"status"`
	Score               int                        `json:"score"`
	Summary             Summary                    `json:"summary"`
	ManufacturerProfile *physicalrules.ProfileInfo `json:"manufacturer_profile,omitempty"`
	Issues              []reports.Issue            `json:"issues"`
	Artifacts           []Artifact                 `json:"artifacts"`
	PhysicalRules       *physicalrules.Report      `json:"physical_rules,omitempty"`
	ManifestPath        string                     `json:"manifest_path,omitempty"`
	DryRun              bool                       `json:"dry_run"`
}

func CalculateStatus(issues []reports.Issue, evidence map[string]EvidenceStatus) ReadinessStatus {
	if reports.HasBlockingIssue(issues) {
		return StatusBlocked
	}
	if len(evidence) == 0 {
		// No evidence means the design can only be previewed. Evaluators should
		// populate expected gates when a project is being judged for readiness.
		return StatusCandidate
	}
	candidate := false
	for _, status := range evidence {
		switch status {
		case EvidenceFail:
			return StatusBlocked
		case EvidenceMissing, EvidenceSkipped, EvidenceWarning:
			candidate = true
		case EvidencePass:
		default:
			return StatusBlocked
		}
	}
	if candidate {
		return StatusCandidate
	}
	return StatusReady
}

func Score(evidence map[string]EvidenceStatus) int {
	if len(evidence) == 0 {
		return 0
	}
	// Readiness score is intentionally an unweighted pass ratio. Gate severity
	// is represented by status and blocking issues rather than score weight.
	var passed int
	for _, status := range evidence {
		if status == EvidencePass {
			passed++
		}
	}
	return (passed*100 + len(evidence)/2) / len(evidence)
}

func NormalizeManifest(manifest Manifest) Manifest {
	manifest.Artifacts = slices.Clone(manifest.Artifacts)
	manifest.Issues = slices.Clone(manifest.Issues)
	if manifest.Artifacts == nil {
		manifest.Artifacts = []Artifact{}
	}
	if manifest.Issues == nil {
		manifest.Issues = []reports.Issue{}
	}
	if strings.TrimSpace(manifest.Schema) == "" {
		manifest.Schema = ManifestSchema
	}
	if manifest.ManufacturerProfile != nil {
		profile := *manifest.ManufacturerProfile
		manifest.ManufacturerProfile = &profile
	}
	if strings.TrimSpace(string(manifest.CreatedBy)) == "" {
		manifest.CreatedBy = GeneratorKiCadAI
	}
	for index := range manifest.Artifacts {
		manifest.Artifacts[index].Issues = slices.Clone(manifest.Artifacts[index].Issues)
		if manifest.Artifacts[index].Issues == nil {
			manifest.Artifacts[index].Issues = []reports.Issue{}
		}
		manifest.Artifacts[index].Files = slices.Clone(manifest.Artifacts[index].Files)
		for fileIndex := range manifest.Artifacts[index].Files {
			manifest.Artifacts[index].Files[fileIndex] = cleanManifestPath(manifest.Artifacts[index].Files[fileIndex])
		}
		slices.Sort(manifest.Artifacts[index].Files)
		manifest.Artifacts[index].Path = cleanManifestPath(manifest.Artifacts[index].Path)
		slices.SortFunc(manifest.Artifacts[index].Issues, compareIssues)
	}
	slices.SortFunc(manifest.Artifacts, compareArtifacts)
	slices.SortFunc(manifest.Issues, compareIssues)
	if manifest.Evidence == nil {
		manifest.Evidence = map[string]EvidenceStatus{}
	} else {
		evidence := make(map[string]EvidenceStatus, len(manifest.Evidence))
		for key, status := range manifest.Evidence {
			evidence[key] = status
		}
		manifest.Evidence = evidence
	}
	return manifest
}

func MarshalManifest(manifest Manifest) ([]byte, error) {
	normalized := NormalizeManifest(manifest)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func ValidateManifest(manifest Manifest) []reports.Issue {
	var issues []reports.Issue
	schema := strings.TrimSpace(manifest.Schema)
	if schema == "" {
		schema = ManifestSchema
	}
	if schema != ManifestSchema {
		issues = append(issues, issue("schema", "unsupported fabrication package schema"))
	}
	if strings.TrimSpace(manifest.Project.Name) == "" {
		issues = append(issues, issue("project.name", "project name is required"))
	}
	for index, artifact := range manifest.Artifacts {
		artifactPath := fmt.Sprintf("artifacts[%d]", index)
		artifactPathValue := cleanManifestPath(artifact.Path)
		if artifactPathValue == "." {
			issues = append(issues, issue(artifactPath+".path", "artifact path is required"))
			continue
		}
		if !cleanManifestPathInsidePackage(artifactPathValue) {
			issues = append(issues, issue(artifactPath+".path", "artifact path must be relative and inside package root"))
		}
		if artifact.Kind == "" {
			issues = append(issues, issue(artifactPath+".kind", "artifact kind is required"))
		} else if !validArtifactKind(artifact.Kind) {
			issues = append(issues, issue(artifactPath+".kind", "unsupported artifact kind"))
		}
		if artifact.Status == "" {
			issues = append(issues, issue(artifactPath+".status", "artifact status is required"))
		} else if !validArtifactStatus(artifact.Status) {
			issues = append(issues, issue(artifactPath+".status", "unsupported artifact status"))
		}
		for fileIndex, file := range artifact.Files {
			cleanedFile := cleanManifestPath(file)
			if !cleanManifestPathInsidePackage(cleanedFile) {
				issues = append(issues, issue(fmt.Sprintf("%s.files[%d]", artifactPath, fileIndex), "artifact file path must be relative and inside package root"))
			}
		}
	}
	for key, status := range manifest.Evidence {
		if strings.TrimSpace(key) == "" {
			issues = append(issues, issue("evidence", "evidence key is required"))
		}
		if !validEvidenceStatus(status) {
			issues = append(issues, issue("evidence."+key, "unsupported evidence status"))
		}
	}
	slices.SortFunc(issues, compareIssues)
	return issues
}

func validArtifactKind(kind ArtifactKind) bool {
	switch kind {
	case ArtifactBOM, ArtifactCPL, ArtifactManifest, ArtifactGerber, ArtifactDrill, ArtifactERC, ArtifactDRC, ArtifactBlockReadiness, ArtifactPhysicalRules, ArtifactReadinessReport:
		return true
	default:
		return false
	}
}

func validArtifactStatus(status ArtifactStatus) bool {
	switch status {
	case ArtifactExpected, ArtifactGenerated, ArtifactMissing, ArtifactSkipped, ArtifactBlocked:
		return true
	default:
		return false
	}
}

func validEvidenceStatus(status EvidenceStatus) bool {
	switch status {
	case EvidencePass, EvidenceWarning, EvidenceMissing, EvidenceSkipped, EvidenceFail:
		return true
	default:
		return false
	}
}

func cleanManifestPath(value string) string {
	slashed := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if slashed == "" {
		return "."
	}
	return path.Clean(slashed)
}

func manifestPathInsidePackage(value string) bool {
	return cleanManifestPathInsidePackage(cleanManifestPath(value))
}

func cleanManifestPathInsidePackage(cleaned string) bool {
	if cleaned == "." || path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return false
	}
	if strings.Contains(cleaned, ":") {
		return false
	}
	return true
}

func ReportArtifacts(artifacts []Artifact) []reports.Artifact {
	out := make([]reports.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, reports.Artifact{
			Kind:        reportsArtifactKind(artifact.Kind),
			Path:        artifact.Path,
			Description: artifact.Description,
		})
	}
	return out
}

func compareArtifacts(a, b Artifact) int {
	if a.Kind != b.Kind {
		return cmp.Compare(string(a.Kind), string(b.Kind))
	}
	if a.Path != b.Path {
		return cmp.Compare(a.Path, b.Path)
	}
	return cmp.Compare(string(a.Status), string(b.Status))
}

func compareIssues(a, b reports.Issue) int {
	if a.Code != b.Code {
		return cmp.Compare(string(a.Code), string(b.Code))
	}
	if a.Severity != b.Severity {
		return cmp.Compare(severityRank(a.Severity), severityRank(b.Severity))
	}
	if a.Path != b.Path {
		return cmp.Compare(a.Path, b.Path)
	}
	return cmp.Compare(a.Message, b.Message)
}

func severityRank(severity reports.Severity) int {
	switch severity {
	case reports.SeverityBlocked:
		return 0
	case reports.SeverityError:
		return 1
	case reports.SeverityWarning:
		return 2
	case reports.SeverityInfo:
		return 3
	default:
		return 4
	}
}

func reportsArtifactKind(kind ArtifactKind) reports.ArtifactKind {
	switch kind {
	case ArtifactBOM:
		return reports.ArtifactBOM
	case ArtifactCPL:
		return reports.ArtifactCPL
	case ArtifactGerber:
		return reports.ArtifactGerber
	case ArtifactDrill:
		return reports.ArtifactDrill
	case ArtifactERC:
		return reports.ArtifactERCReport
	case ArtifactDRC:
		return reports.ArtifactDRCReport
	case ArtifactBlockReadiness:
		return reports.ArtifactPromotionReport
	case ArtifactManifest, ArtifactPhysicalRules, ArtifactReadinessReport:
		return reports.ArtifactFabricationPackage
	default:
		return reports.ArtifactFabricationPackage
	}
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}
