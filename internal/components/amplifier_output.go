package components

import (
	"context"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const CodeAmplifierOutputUnsupported reports.Code = "AMPLIFIER_OUTPUT_UNSUPPORTED"

const maxHeadphoneSeedPeakCurrentMA = 200
const maxAmplifierOutputRejectedDetails = 5

type AmplifierOutputPairRequest struct {
	SupplyVoltage    string          `json:"supply_voltage,omitempty"`
	SupplyUnit       string          `json:"supply_unit,omitempty"`
	LoadImpedance    string          `json:"load_impedance,omitempty"`
	LoadUnit         string          `json:"load_unit,omitempty"`
	Acceptance       AcceptanceLevel `json:"acceptance,omitempty"`
	RequireHeadphone bool            `json:"require_headphone,omitempty"`
}

type AmplifierOutputPair struct {
	NPN             Selection `json:"npn"`
	PNP             Selection `json:"pnp"`
	EstimatedPeakMA string    `json:"estimated_peak_ma,omitempty"`
}

func SelectAmplifierOutputPair(ctx context.Context, catalog *Catalog, request AmplifierOutputPairRequest) (AmplifierOutputPair, reports.Result) {
	if err := ctx.Err(); err != nil {
		issue := reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Message: err.Error()}
		return AmplifierOutputPair{}, reports.ErrorResult("amplifier output pair select", issue)
	}
	if catalog == nil {
		issue := NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "catalog", "component catalog is nil")
		return AmplifierOutputPair{}, reports.ErrorResult("amplifier output pair select", issue)
	}
	if request.Acceptance == "" {
		request.Acceptance = AcceptanceConnectivity
	}
	requiredRatings, estimatedPeakMA, issues := amplifierOutputRequiredRatings(request)
	if len(issues) > 0 {
		pair := AmplifierOutputPair{EstimatedPeakMA: estimatedPeakMA}
		return pair, reports.ResultWithIssues("amplifier output pair select", pair, issues, nil)
	}
	npn, npnIssues := selectAmplifierOutputPolarity(catalog, "npn", request, requiredRatings)
	pnp, pnpIssues := selectAmplifierOutputPolarity(catalog, "pnp", request, requiredRatings)
	issues = append(issues, npnIssues...)
	issues = append(issues, pnpIssues...)
	pair := AmplifierOutputPair{NPN: npn, PNP: pnp, EstimatedPeakMA: estimatedPeakMA}
	issues = append(issues, validateComplementaryOutputPair(pair)...)
	return pair, reports.ResultWithIssues("amplifier output pair select", pair, issues, nil)
}

func amplifierOutputRequiredRatings(request AmplifierOutputPairRequest) ([]RequiredRating, string, []reports.Issue) {
	var issues []reports.Issue
	voltage := strings.TrimSpace(request.SupplyVoltage)
	voltageUnit := strings.TrimSpace(request.SupplyUnit)
	if voltageUnit == "" {
		voltageUnit = "V"
	}
	load := strings.TrimSpace(request.LoadImpedance)
	loadUnit := strings.TrimSpace(request.LoadUnit)
	if loadUnit == "" {
		loadUnit = "ohm"
	}
	if voltage == "" {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.supply_voltage", "supply voltage is required for amplifier output selection"))
	}
	if load == "" {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.load_impedance", "load impedance is required for amplifier output selection"))
	}
	if len(issues) > 0 {
		return nil, "", issues
	}
	supplyValue, supplyOK := parseValueWithUnit(voltage, voltageUnit)
	loadValue, loadOK := parseValueWithUnit(load, loadUnit)
	if !supplyOK {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.supply_voltage", "supply voltage cannot be parsed: "+voltage+" "+voltageUnit))
	}
	if !loadOK {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.load_impedance", "load impedance cannot be parsed: "+load+" "+loadUnit))
	}
	if supplyOK && supplyValue <= 0 {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.supply_voltage", "supply voltage must be greater than zero"))
	}
	if loadOK && loadValue <= 0 {
		issues = append(issues, NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "amplifier_output.load_impedance", "load impedance must be greater than zero"))
	}
	if len(issues) > 0 {
		return nil, "", issues
	}
	if request.RequireHeadphone && loadValue < 16 {
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.load_impedance", "speaker or low-impedance power-amplifier loads are not supported by the headphone output-stage selector"))
	}
	peakCurrentA := supplyValue / (2 * loadValue)
	peakCurrentMA := peakCurrentA * 1000
	if peakCurrentMA > maxHeadphoneSeedPeakCurrentMA {
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.estimated_peak_current", "estimated output current exceeds supported low-current headphone seed devices"))
	}
	if len(issues) > 0 {
		return nil, formatMilliAmp(peakCurrentMA), issues
	}
	return []RequiredRating{
		{Kind: "collector_emitter_voltage", Value: strconv.FormatFloat(supplyValue, 'g', 8, 64), Unit: "V"},
		{Kind: "collector_current", Value: formatMilliAmp(peakCurrentMA), Unit: "mA"},
	}, formatMilliAmp(peakCurrentMA), nil
}

func selectAmplifierOutputPolarity(catalog *Catalog, polarity string, request AmplifierOutputPairRequest, ratings []RequiredRating) (Selection, []reports.Issue) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	candidates := catalog.amplifierOutputIndex[polarity]
	var rejected []CandidateRejection
	for _, candidate := range candidates {
		if candidate.Record < 0 || candidate.Record >= len(catalog.Records) {
			continue
		}
		record := &catalog.Records[candidate.Record]
		if candidate.Variant < 0 || candidate.Variant >= len(record.Packages) {
			continue
		}
		variant := &record.Packages[candidate.Variant]
		issues := requiredRatingIssues(*record, ratings)
		issues = append(issues, selectionCandidateIssues(*record, candidate.Candidate, SelectionRequest{
			Acceptance:        request.Acceptance,
			RequireConcrete:   true,
			RequireCompanions: true,
		})...)
		if request.Acceptance == AcceptanceFabricationCandidate {
			issues = append(issues, fabricationCandidateReviewIssues(*record)...)
		}
		if reports.HasBlockingIssue(issues) {
			if len(rejected) < maxAmplifierOutputRejectedDetails {
				rejected = append(rejected, CandidateRejection{Candidate: candidate.Candidate, Issues: issues})
			}
			continue
		}
		warnings := nonBlockingIssues(issues)
		return Selection{
			Candidate: candidate.Candidate,
			Component: *record,
			Variant:   *variant,
			Warnings:  warnings,
			Rejected:  rejected,
		}, warnings
	}
	issues := []reports.Issue{NewIssue(CodeComponentNotFound, reports.SeverityBlocked, "amplifier_output."+polarity, "no supported "+polarity+" headphone output device satisfies requested ratings")}
	for _, rejection := range rejected {
		issues = append(issues, rejection.Issues...)
	}
	return Selection{Rejected: rejected}, issues
}

func validateComplementaryOutputPair(pair AmplifierOutputPair) []reports.Issue {
	if pair.NPN.Component.AmplifierOutput == nil || pair.PNP.Component.AmplifierOutput == nil {
		return nil
	}
	npnGroup := strings.TrimSpace(pair.NPN.Component.AmplifierOutput.ComplementaryGroup)
	pnpGroup := strings.TrimSpace(pair.PNP.Component.AmplifierOutput.ComplementaryGroup)
	if npnGroup == "" || pnpGroup == "" {
		return []reports.Issue{NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.complementary_group", "selected output devices require complementary_group metadata")}
	}
	if !strings.EqualFold(npnGroup, pnpGroup) {
		return []reports.Issue{NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.complementary_group", "selected NPN and PNP output devices are not in the same complementary group")}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func formatMilliAmp(value float64) string {
	return strconv.FormatFloat(value, 'g', 8, 64)
}
