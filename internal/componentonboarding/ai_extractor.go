package componentonboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"kicadai/internal/aiprovider"
)

// AIExtractor adapts the project's structured-output provider boundary to
// component evidence extraction. Provider output remains untrusted and is
// subject to every deterministic check in Onboard.
type AIExtractor struct {
	Provider          aiprovider.Provider
	OutputSchema      map[string]any
	MaxOutputTokens   int
	CapabilityContext string
}

func (extractor AIExtractor) Extract(
	ctx context.Context,
	requirement BehavioralRequirement,
	documents []DocumentInput,
) (Extraction, error) {
	if extractor.Provider == nil || len(extractor.OutputSchema) == 0 {
		return Extraction{}, fmt.Errorf("AI extractor requires a provider and strict output schema")
	}
	tokenLimit := extractor.MaxOutputTokens
	if tokenLimit == 0 {
		tokenLimit = aiprovider.DefaultGenericOutputTokens
	}
	prompt, err := extractionPrompt(requirement, documents)
	if err != nil {
		return Extraction{}, err
	}
	capability := strings.TrimSpace(extractor.CapabilityContext)
	if capability == "" {
		capability = "Extract candidate component evidence only from supplied immutable manufacturer documents. Preserve exact excerpts and locations. Do not infer missing ratings, pins, packages, models, derating rules, or provenance. Return schema " + ExtractionSchema + "."
	}
	result, err := extractor.Provider.GenerateIntent(ctx, aiprovider.GenerateRequest{
		Prompt: prompt, CapabilityContext: capability,
		OutputSchemaName: "component_onboarding_extraction",
		OutputSchema:     extractor.OutputSchema, SchemaVersion: ExtractionSchema,
		Attempt: 1, MaxOutputTokens: tokenLimit,
	})
	if err != nil {
		return Extraction{}, err
	}
	extraction, err := DecodeExtraction(bytes.NewReader(result.IntentJSON))
	if err != nil {
		return Extraction{}, fmt.Errorf("decode AI component extraction: %w", err)
	}
	return extraction, nil
}

type extractionPromptDocument struct {
	ID        string       `json:"id"`
	Kind      DocumentKind `json:"kind"`
	Publisher string       `json:"publisher"`
	Locator   string       `json:"locator"`
	Revision  string       `json:"revision"`
	License   string       `json:"license,omitempty"`
	SHA256    string       `json:"sha256"`
	Content   string       `json:"content"`
}

type extractionPromptEnvelope struct {
	Task        string                     `json:"task"`
	Requirement BehavioralRequirement      `json:"requirement"`
	Documents   []extractionPromptDocument `json:"documents"`
}

func extractionPrompt(requirement BehavioralRequirement, documents []DocumentInput) (string, error) {
	const task = "Discover and extract all source-supported component candidates satisfying the behavioral requirement."
	requirementJSON, err := json.Marshal(requirement)
	if err != nil {
		return "", err
	}
	inputBytes := int64(len(task) + len(requirementJSON))
	addInputBytes := func(size int) error {
		if int64(size) > int64(aiprovider.MaxPromptBytes)-inputBytes {
			return fmt.Errorf("component extraction input exceeds safe %d-byte pre-marshal bound", aiprovider.MaxPromptBytes)
		}
		inputBytes += int64(size)
		return nil
	}
	envelope := extractionPromptEnvelope{
		Task:        task,
		Requirement: requirement,
	}
	for _, document := range documents {
		for _, value := range []string{
			document.ID, string(document.Kind), document.Publisher, document.Locator,
			document.Revision, document.License, document.ExpectedSHA256,
		} {
			if err := addInputBytes(len(value)); err != nil {
				return "", err
			}
		}
		if err := addInputBytes(len(document.Content)); err != nil {
			return "", err
		}
		if !utf8.Valid(document.Content) {
			return "", fmt.Errorf("document %q is not UTF-8 text; preprocess binary documentation into a content-addressed text representation", document.ID)
		}
		envelope.Documents = append(envelope.Documents, extractionPromptDocument{
			ID: document.ID, Kind: document.Kind, Publisher: document.Publisher,
			Locator: document.Locator, Revision: document.Revision, License: document.License,
			SHA256: document.ExpectedSHA256, Content: string(document.Content),
		})
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if len(data) > aiprovider.MaxPromptBytes {
		return "", fmt.Errorf("component extraction prompt exceeds %d-byte provider bound", aiprovider.MaxPromptBytes)
	}
	return string(data), nil
}
