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
	DeviceClass      string          `json:"device_class,omitempty"`
	Application      string          `json:"application,omitempty"`
	SupplyVoltage    string          `json:"supply_voltage,omitempty"`
	SupplyUnit       string          `json:"supply_unit,omitempty"`
	LoadImpedance    string          `json:"load_impedance,omitempty"`
	LoadUnit         string          `json:"load_unit,omitempty"`
	Acceptance       AcceptanceLevel `json:"acceptance,omitempty"`
	RequireHeadphone bool            `json:"require_headphone,omitempty"`
}

type AmplifierOutputPair struct {
	DeviceClass     string    `json:"device_class,omitempty"`
	Upper           Selection `json:"upper"`
	Lower           Selection `json:"lower"`
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
	if request.DeviceClass == "" {
		request.DeviceClass = "bjt"
	}
	if request.Application == "" {
		request.Application = "headphone"
	}
	requiredRatings, estimatedPeakMA, issues := amplifierOutputRequiredRatings(request)
	if len(issues) > 0 {
		pair := AmplifierOutputPair{EstimatedPeakMA: estimatedPeakMA}
		return pair, reports.ResultWithIssues("amplifier output pair select", pair, issues, nil)
	}
	upperPolarity, lowerPolarity := "npn", "pnp"
	if request.DeviceClass == "mosfet" {
		upperPolarity, lowerPolarity = "n_channel", "p_channel"
	}
	upperCandidates, upperRejected := selectAmplifierOutputPolarityCandidates(catalog, upperPolarity, request, requiredRatings)
	lowerCandidates, lowerRejected := selectAmplifierOutputPolarityCandidates(catalog, lowerPolarity, request, requiredRatings)
	if len(upperCandidates) == 0 {
		issues = append(issues, amplifierOutputPolarityIssues(upperPolarity, request.Application, upperRejected)...)
	}
	if len(lowerCandidates) == 0 {
		issues = append(issues, amplifierOutputPolarityIssues(lowerPolarity, request.Application, lowerRejected)...)
	}
	upper, lower, paired := bestComplementaryOutputPair(upperCandidates, lowerCandidates)
	if !paired && len(upperCandidates) != 0 && len(lowerCandidates) != 0 {
		upper, lower = upperCandidates[0], lowerCandidates[0]
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.complementary_group", "no jointly valid output devices share a complementary group"))
	}
	pair := AmplifierOutputPair{DeviceClass: request.DeviceClass, Upper: upper, Lower: lower, EstimatedPeakMA: estimatedPeakMA}
	if request.DeviceClass == "bjt" {
		pair.NPN, pair.PNP = upper, lower
	}
	issues = append(issues, upper.Warnings...)
	issues = append(issues, lower.Warnings...)
	if paired {
		issues = append(issues, validateComplementaryOutputPair(pair)...)
	}
	return pair, reports.ResultWithIssues("amplifier output pair select", pair, issues, nil)
}

func amplifierOutputRequiredRatings(request AmplifierOutputPairRequest) ([]RequiredRating, string, []reports.Issue) {
	var issues []reports.Issue
	switch request.DeviceClass {
	case "bjt", "mosfet":
	default:
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.device_class", "device class must be bjt or mosfet"))
	}
	switch request.Application {
	case "headphone", "power":
	default:
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.application", "application must be headphone or power"))
	}
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
	if (request.RequireHeadphone || request.Application == "headphone") && loadValue < 16 {
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.load_impedance", "speaker or low-impedance power-amplifier loads are not supported by the headphone output-stage selector"))
	}
	peakCurrentA := supplyValue / (2 * loadValue)
	peakCurrentMA := peakCurrentA * 1000
	if request.Application == "headphone" && peakCurrentMA > maxHeadphoneSeedPeakCurrentMA {
		issues = append(issues, NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.estimated_peak_current", "estimated output current exceeds supported low-current headphone seed devices"))
	}
	if len(issues) > 0 {
		return nil, formatMilliAmp(peakCurrentMA), issues
	}
	if request.DeviceClass == "mosfet" {
		return []RequiredRating{
			{Kind: "drain_source_voltage", Value: strconv.FormatFloat(supplyValue, 'g', 8, 64), Unit: "V"},
			{Kind: "drain_current", Value: formatMilliAmp(peakCurrentMA), Unit: "mA"},
		}, formatMilliAmp(peakCurrentMA), nil
	}
	return []RequiredRating{{Kind: "collector_emitter_voltage", Value: strconv.FormatFloat(supplyValue, 'g', 8, 64), Unit: "V"}, {Kind: "collector_current", Value: formatMilliAmp(peakCurrentMA), Unit: "mA"}}, formatMilliAmp(peakCurrentMA), nil
}

func selectAmplifierOutputPolarityCandidates(catalog *Catalog, polarity string, request AmplifierOutputPairRequest, ratings []RequiredRating) ([]Selection, []CandidateRejection) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	candidates := catalog.amplifierOutputIndex[polarity]
	var accepted []Selection
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
		if record.AmplifierOutput == nil || record.AmplifierOutput.DeviceClass != request.DeviceClass || !amplifierOutputSupportsApplication(record.AmplifierOutput, request.Application) {
			continue
		}
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
		accepted = append(accepted, Selection{
			Candidate: candidate.Candidate,
			Component: *record,
			Variant:   *variant,
			Warnings:  warnings,
			Rejected:  rejected,
		})
	}
	return accepted, rejected
}

func amplifierOutputPolarityIssues(polarity string, application string, rejected []CandidateRejection) []reports.Issue {
	issues := []reports.Issue{NewIssue(CodeComponentNotFound, reports.SeverityBlocked, "amplifier_output."+polarity, "no supported "+polarity+" "+application+" output device satisfies requested ratings")}
	for _, rejection := range rejected {
		issues = append(issues, rejection.Issues...)
	}
	return issues
}

func bestComplementaryOutputPair(uppers []Selection, lowers []Selection) (Selection, Selection, bool) {
	var bestUpper Selection
	var bestLower Selection
	bestScore := 0
	found := false
	for _, upper := range uppers {
		if upper.Component.AmplifierOutput == nil {
			continue
		}
		upperGroup := strings.TrimSpace(upper.Component.AmplifierOutput.ComplementaryGroup)
		if upperGroup == "" {
			continue
		}
		for _, lower := range lowers {
			if lower.Component.AmplifierOutput == nil ||
				!strings.EqualFold(upperGroup, strings.TrimSpace(lower.Component.AmplifierOutput.ComplementaryGroup)) {
				continue
			}
			score := upper.Candidate.Score + lower.Candidate.Score
			if !found || score > bestScore ||
				score == bestScore && amplifierOutputPairKey(upper, lower) < amplifierOutputPairKey(bestUpper, bestLower) {
				bestUpper, bestLower, bestScore, found = upper, lower, score, true
			}
		}
	}
	return bestUpper, bestLower, found
}

func amplifierOutputPairKey(upper Selection, lower Selection) string {
	return upper.Component.ID + "\x00" + upper.Variant.ID + "\x00" + lower.Component.ID + "\x00" + lower.Variant.ID
}

func validateComplementaryOutputPair(pair AmplifierOutputPair) []reports.Issue {
	upper, lower := pair.Upper, pair.Lower
	if upper.Component.AmplifierOutput == nil || lower.Component.AmplifierOutput == nil {
		return nil
	}
	upperGroup := strings.TrimSpace(upper.Component.AmplifierOutput.ComplementaryGroup)
	lowerGroup := strings.TrimSpace(lower.Component.AmplifierOutput.ComplementaryGroup)
	if upperGroup == "" || lowerGroup == "" {
		return []reports.Issue{NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.complementary_group", "selected output devices require complementary_group metadata")}
	}
	if !strings.EqualFold(upperGroup, lowerGroup) {
		return []reports.Issue{NewIssue(CodeAmplifierOutputUnsupported, reports.SeverityBlocked, "amplifier_output.complementary_group", "selected NPN and PNP output devices are not in the same complementary group")}
	}
	return nil
}

func amplifierOutputSupportsApplication(evidence *AmplifierOutputEvidence, application string) bool {
	if evidence == nil {
		return false
	}
	if application == "power" {
		return containsString(evidence.IntendedRoles, "power_output")
	}
	return containsString(evidence.IntendedRoles, "headphone_output")
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
