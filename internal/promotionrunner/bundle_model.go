package promotionrunner

import "kicadai/internal/promotiontoolchain"

const (
	BundleSchema             = "kicadai.clean-checkout-promotion.v1"
	BundleToolchainSchema    = "kicadai.promotion-bundle-toolchain.v1"
	BundleCommandsSchema     = "kicadai.promotion-bundle-commands.v1"
	BundleVerificationSchema = "kicadai.promotion-bundle-verification.v1"
)

type BundleFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type BundleIdentity struct {
	Schema string `json:"schema"`
	SHA256 string `json:"sha256"`
}

type BundleReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type BundleToolchain struct {
	Schema               string                             `json:"schema"`
	LockSHA256           string                             `json:"lock_sha256"`
	OS                   string                             `json:"os"`
	Arch                 string                             `json:"arch"`
	KiCadVersion         string                             `json:"kicad_version"`
	SymbolTableSHA256    string                             `json:"symbol_table_sha256"`
	FootprintTableSHA256 string                             `json:"footprint_table_sha256"`
	SymbolsIdentity      promotiontoolchain.LibraryIdentity `json:"symbols_identity"`
	FootprintsIdentity   promotiontoolchain.LibraryIdentity `json:"footprints_identity"`
}

type BundleCommand struct {
	Scenario    string            `json:"scenario"`
	Run         int               `json:"run"`
	Args        []string          `json:"args"`
	Environment map[string]string `json:"environment"`
	Status      string            `json:"status"`
}

type BundleCommands struct {
	Schema  string          `json:"schema"`
	Records []BundleCommand `json:"records"`
}

type BundleRun struct {
	Run           int    `json:"run"`
	Status        string `json:"status"`
	ProjectSHA256 string `json:"project_sha256"`
	PromotionPath string `json:"promotion_path"`
}

type BundleScenario struct {
	ID         string          `json:"id"`
	Lane       string          `json:"lane"`
	Status     string          `json:"status"`
	Request    BundleReference `json:"request"`
	Runs       []BundleRun     `json:"runs"`
	Comparison BundleReference `json:"comparison"`
}

type BundleManifest struct {
	Schema             string           `json:"schema"`
	Status             string           `json:"status"`
	RepositoryRevision string           `json:"repository_revision"`
	Matrix             BundleIdentity   `json:"matrix"`
	LaneRegistry       BundleIdentity   `json:"lane_registry"`
	Toolchain          BundleReference  `json:"toolchain"`
	Commands           BundleReference  `json:"commands"`
	Scenarios          []BundleScenario `json:"scenarios"`
	Files              []BundleFile     `json:"files"`
}

type BundleBuildOptions struct {
	RepositoryRoot     string
	PromotionRoot      string
	DestinationParent  string
	RepositoryRevision string
	Matrix             MatrixDocument
	Toolchain          promotiontoolchain.Evidence
	Results            []RunResult
}

type BundleResult struct {
	Schema         string `json:"schema"`
	Status         string `json:"status"`
	Path           string `json:"path"`
	ManifestSHA256 string `json:"manifest_sha256"`
	FileCount      int    `json:"file_count"`
}

type BundleVerification struct {
	Schema         string `json:"schema"`
	Status         string `json:"status"`
	Bundle         string `json:"bundle"`
	ManifestSHA256 string `json:"manifest_sha256"`
	FileCount      int    `json:"file_count"`
}
