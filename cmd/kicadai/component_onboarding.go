package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/componentonboarding"
	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
)

const (
	onboardingBuildRequestSchema     = "kicadai.component-onboarding-build-request.v1"
	onboardingPromotionRequestSchema = "kicadai.component-onboarding-promotion-request.v1"
)

type onboardingDocumentFile struct {
	ID             string                           `json:"id"`
	Kind           componentonboarding.DocumentKind `json:"kind"`
	Publisher      string                           `json:"publisher"`
	Locator        string                           `json:"locator"`
	Revision       string                           `json:"revision"`
	License        string                           `json:"license,omitempty"`
	Path           string                           `json:"path"`
	ExpectedSHA256 string                           `json:"expected_sha256"`
}

type onboardingBuildRequest struct {
	Schema       string                                    `json:"schema"`
	Requirement  componentonboarding.BehavioralRequirement `json:"requirement"`
	Documents    []onboardingDocumentFile                  `json:"documents"`
	OutputSchema map[string]any                            `json:"output_schema,omitempty"`
}

type onboardingPromotionRequest struct {
	Schema    string                             `json:"schema"`
	Documents []onboardingDocumentFile           `json:"documents"`
	Gates     []componentonboarding.GateEvidence `json:"gates"`
	Approval  componentonboarding.Approval       `json:"approval"`
}

type fileComponentExtractor struct {
	extraction componentonboarding.Extraction
}

func (extractor fileComponentExtractor) Extract(
	context.Context,
	componentonboarding.BehavioralRequirement,
	[]componentonboarding.DocumentInput,
) (componentonboarding.Extraction, error) {
	return extractor.extraction, nil
}

func runComponentOnboard(
	ctx context.Context,
	opts cliOptions,
	stdout io.Writer,
	catalog *components.Catalog,
) error {
	if strings.TrimSpace(opts.requestPath) == "" || strings.TrimSpace(opts.output) == "" {
		return componentOnboardingFailure(stdout, "component.onboard", "component onboard requires --request and --output")
	}
	request, err := decodeCapabilityFile[onboardingBuildRequest](opts.requestPath)
	if err != nil {
		return componentOnboardingFailure(stdout, opts.requestPath, err.Error())
	}
	if request.Schema != onboardingBuildRequestSchema {
		return componentOnboardingFailure(stdout, "schema", "invalid component onboarding build-request schema")
	}
	documents, err := loadOnboardingDocuments(filepath.Dir(opts.requestPath), request.Documents)
	if err != nil {
		return componentOnboardingFailure(stdout, "documents", err.Error())
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return componentOnboardingFailure(stdout, "libraries", err.Error())
	}
	var extractor componentonboarding.Extractor
	if strings.TrimSpace(opts.intentFile) != "" {
		extraction, err := decodeOnboardingArtifactFile(opts.intentFile, componentonboarding.DecodeExtraction)
		if err != nil {
			return componentOnboardingFailure(stdout, opts.intentFile, err.Error())
		}
		extractor = fileComponentExtractor{extraction: extraction}
	} else {
		provider, err := aiProviderFromOptions(opts)
		if err != nil {
			return componentOnboardingFailure(stdout, "provider", err.Error())
		}
		if len(request.OutputSchema) == 0 {
			return componentOnboardingFailure(stdout, "output_schema", "AI onboarding requires a strict extraction output schema in the build request")
		}
		extractor = componentonboarding.AIExtractor{
			Provider: provider, OutputSchema: request.OutputSchema,
			MaxOutputTokens: opts.aiMaxOutputTokens,
		}
	}
	candidate, err := componentonboarding.Onboard(ctx, request.Requirement, documents, extractor, catalog, index)
	if err != nil {
		return componentOnboardingFailure(stdout, "component.onboard", err.Error())
	}
	if err := componentonboarding.WriteArtifact(opts.output, candidate); err != nil {
		return componentOnboardingFailure(stdout, opts.output, err.Error())
	}
	return writeComponentResult(stdout, candidate, nil)
}

func runComponentOnboardingPromote(
	ctx context.Context,
	opts cliOptions,
	stdout io.Writer,
	catalog *components.Catalog,
) error {
	if !opts.execute {
		return componentOnboardingFailure(stdout, "component.promote", "component promote requires explicit --execute authorization")
	}
	if strings.TrimSpace(opts.requestPath) == "" || strings.TrimSpace(opts.intentFile) == "" ||
		strings.TrimSpace(opts.output) == "" {
		return componentOnboardingFailure(stdout, "component.promote", "component promote requires --request candidate, --file promotion request, and --output overlay")
	}
	candidate, err := decodeOnboardingArtifactFile(opts.requestPath, componentonboarding.DecodeCandidate)
	if err != nil {
		return componentOnboardingFailure(stdout, opts.requestPath, err.Error())
	}
	request, err := decodeCapabilityFile[onboardingPromotionRequest](opts.intentFile)
	if err != nil {
		return componentOnboardingFailure(stdout, opts.intentFile, err.Error())
	}
	if request.Schema != onboardingPromotionRequestSchema {
		return componentOnboardingFailure(stdout, "schema", "invalid component onboarding promotion-request schema")
	}
	documents, err := loadOnboardingDocuments(filepath.Dir(opts.intentFile), request.Documents)
	if err != nil {
		return componentOnboardingFailure(stdout, "documents", err.Error())
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return componentOnboardingFailure(stdout, "libraries", err.Error())
	}
	promotion, overlay, err := componentonboarding.Promote(
		candidate, documents, request.Gates, request.Approval, catalog, index,
	)
	if err != nil {
		return componentOnboardingFailure(stdout, "component.promote", err.Error())
	}
	if err := componentonboarding.WriteArtifact(opts.output, overlay); err != nil {
		return componentOnboardingFailure(stdout, opts.output, err.Error())
	}
	if strings.TrimSpace(opts.target) != "" {
		if err := componentonboarding.WriteArtifact(opts.target, promotion); err != nil {
			return componentOnboardingFailure(stdout, opts.target, err.Error())
		}
	}
	return writeComponentResult(stdout, map[string]any{
		"promotion": promotion, "overlay": overlay,
	}, nil)
}

func runComponentOverlayValidate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return componentOnboardingFailure(stdout, "component.overlay-validate", "component overlay-validate requires --request overlay")
	}
	base, err := loadComponentCatalog(ctx, opts.catalogDir)
	if err != nil {
		return componentOnboardingFailure(stdout, "catalog-dir", err.Error())
	}
	overlay, err := decodeOnboardingArtifactFile(opts.requestPath, componentonboarding.DecodeOverlay)
	if err != nil {
		return componentOnboardingFailure(stdout, opts.requestPath, err.Error())
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return componentOnboardingFailure(stdout, "models", diagnostics[0].Message)
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return componentOnboardingFailure(stdout, "libraries", err.Error())
	}
	catalog, mergedModels, err := componentonboarding.ApplyOverlay(base, models, overlay, index)
	if err != nil {
		return componentOnboardingFailure(stdout, "component.overlay-validate", err.Error())
	}
	modelHash, err := modelprovenance.Hash(mergedModels)
	if err != nil {
		return componentOnboardingFailure(stdout, "models", err.Error())
	}
	return writeComponentResult(stdout, map[string]any{
		"overlay_hash": overlay.Hash, "catalog_records": len(catalog.Records),
		"model_records": len(mergedModels.Records), "model_registry_hash": modelHash,
	}, nil)
}

func loadComponentCatalogForOptions(ctx context.Context, opts cliOptions) (*components.Catalog, error) {
	base, err := loadComponentCatalog(ctx, opts.catalogDir)
	if err != nil || strings.TrimSpace(opts.componentOverlay) == "" {
		return base, err
	}
	overlay, err := decodeOnboardingArtifactFile(opts.componentOverlay, componentonboarding.DecodeOverlay)
	if err != nil {
		return nil, err
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("load model provenance: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return nil, err
	}
	catalog, _, err := componentonboarding.ApplyOverlay(base, models, overlay, index)
	return catalog, err
}

func loadComponentModelsForOptions(ctx context.Context, opts cliOptions) (modelprovenance.Registry, error) {
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return modelprovenance.Registry{}, fmt.Errorf("load model provenance: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	if strings.TrimSpace(opts.componentOverlay) == "" {
		return models, nil
	}
	base, err := loadComponentCatalog(ctx, opts.catalogDir)
	if err != nil {
		return modelprovenance.Registry{}, err
	}
	overlay, err := decodeOnboardingArtifactFile(opts.componentOverlay, componentonboarding.DecodeOverlay)
	if err != nil {
		return modelprovenance.Registry{}, err
	}
	index, err := componentOnboardingLibraryIndex(ctx, opts)
	if err != nil {
		return modelprovenance.Registry{}, err
	}
	_, merged, err := componentonboarding.ApplyOverlay(base, models, overlay, index)
	return merged, err
}

func componentOnboardingLibraryIndex(ctx context.Context, opts cliOptions) (libraryresolver.LibraryIndex, error) {
	roots := libraryRootsFromOptions(opts)
	if strings.TrimSpace(roots.SymbolsRoot) == "" || strings.TrimSpace(roots.FootprintsRoot) == "" {
		return libraryresolver.LibraryIndex{}, fmt.Errorf("component onboarding requires symbols-root and footprints-root")
	}
	index, _ := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{
		CachePath: opts.libraryCache, Refresh: opts.refreshLibraryCache,
	})
	return index, nil
}

func loadOnboardingDocuments(baseDir string, inputs []onboardingDocumentFile) ([]componentonboarding.DocumentInput, error) {
	documents := make([]componentonboarding.DocumentInput, 0, len(inputs))
	for _, input := range inputs {
		path, err := resolveCapabilitySourcePath(baseDir, input.Path)
		if err != nil {
			return nil, err
		}
		content, err := readCapabilitySource(path)
		if err != nil {
			return nil, err
		}
		documents = append(documents, componentonboarding.DocumentInput{
			ID: input.ID, Kind: input.Kind, Publisher: input.Publisher,
			Locator: input.Locator, Revision: input.Revision, License: input.License,
			ExpectedSHA256: input.ExpectedSHA256, Content: content,
		})
	}
	return documents, nil
}

func decodeOnboardingArtifactFile[T any](path string, decoder func(io.Reader) (T, error)) (T, error) {
	var zero T
	file, err := os.Open(path)
	if err != nil {
		return zero, err
	}
	defer func() { _ = file.Close() }()
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(file, maxCapabilityRequestBytes+1)); err != nil {
		return zero, err
	}
	if buffer.Len() > maxCapabilityRequestBytes {
		return zero, fmt.Errorf("component onboarding artifact exceeds %d-byte limit", maxCapabilityRequestBytes)
	}
	return decoder(bytes.NewReader(buffer.Bytes()))
}

func componentOnboardingFailure(stdout io.Writer, path, message string) error {
	return writeReportFailure(stdout, "component.onboarding", reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
		Path: path, Message: message,
	})
}
