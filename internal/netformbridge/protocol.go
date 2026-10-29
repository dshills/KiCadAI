// Package netformbridge is the versioned local CLI/JSON boundary for NetForm.
// No network service, provider, plugin execution or simulation engine is added.
package netformbridge

import (
	"context"
	"encoding/json"
	"errors"
	"kicadai/internal/boardfamily"
	"kicadai/internal/buildinfo"
	"kicadai/internal/capabilityexplorer"
	"kicadai/internal/componentonboarding"
	"kicadai/internal/components"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/numericalconformance"
	"kicadai/internal/pinmap"
	"kicadai/internal/reproduction"
	"kicadai/internal/schematicassistant"
	"kicadai/internal/schematicreview"
	"regexp"
	"sort"
	"strings"
)

const RequestSchema = "netform.kicadai.request.v1"
const ResponseSchema = "netform.kicadai.response.v1"
const MaxRequest = 64 << 10
const MaxResponse = 2 << 20

var requestID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var Methods = []string{"capabilities", "catalog.get", "catalog.search", "evidence.verify", "family.check", "pinmap.get", "schematic.inspect"}

type Request struct {
	Schema string          `json:"schema"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	Schema string          `json:"schema"`
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}
type CatalogEntry struct {
	ID           string                        `json:"id"`
	Family       string                        `json:"family"`
	Name         string                        `json:"name"`
	MPN          string                        `json:"mpn,omitempty"`
	Generic      bool                          `json:"generic"`
	Symbols      []components.SymbolBinding    `json:"symbols"`
	Packages     []components.PackageVariant   `json:"packages"`
	Values       []components.ValueConstraint  `json:"values"`
	Ratings      []components.RatingConstraint `json:"ratings"`
	Verification components.VerificationRecord `json:"verification"`
}

func entry(r components.ComponentRecord) CatalogEntry {
	return CatalogEntry{r.ID, r.Family, r.Name, r.MPN, r.Generic, r.Symbols, r.Packages, r.Values, r.Ratings, r.Verification}
}
func Handle(ctx context.Context, raw []byte) Response {
	response := Response{Schema: ResponseSchema}
	fail := func(code string, err error) Response {
		response.OK = false
		response.Result = nil
		response.Error = &Error{code, err.Error()}
		return response
	}
	if len(raw) > MaxRequest {
		return fail("REQUEST_TOO_LARGE", errors.New("request exceeds 64 KiB"))
	}
	var request Request
	if err := evidencebundle.Decode(raw, &request); err != nil {
		return fail("INVALID_REQUEST", err)
	}
	if request.Schema != RequestSchema || !requestID.MatchString(request.ID) {
		return fail("INVALID_REQUEST", errors.New("unsupported schema or invalid request ID"))
	}
	response.ID = request.ID
	if err := ctx.Err(); err != nil {
		return fail("CANCELLED", err)
	}
	decode := func(params any) error {
		if len(request.Params) == 0 {
			return errors.New("params must be an explicit object")
		}
		if string(request.Params) == "null" {
			return errors.New("params must not be null")
		}
		return evidencebundle.Decode(request.Params, params)
	}
	var result any
	switch request.Method {
	case "capabilities":
		var params struct{}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		result = struct {
			Build       buildinfo.Info               `json:"build"`
			Methods     []string                     `json:"methods"`
			Workflows   capabilityexplorer.Inventory `json:"workflows"`
			MaxRequest  int                          `json:"max_request_bytes"`
			MaxResponse int                          `json:"max_response_bytes"`
		}{buildinfo.Current(), Methods, capabilityexplorer.Build(), MaxRequest, MaxResponse}
	case "catalog.get", "catalog.search":
		var params struct {
			ID     string `json:"id,omitempty"`
			Query  string `json:"query,omitempty"`
			Family string `json:"family,omitempty"`
			Limit  int    `json:"limit,omitempty"`
		}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		if len(params.Query) > 128 || len(params.ID) > 256 || len(params.Family) > 64 || params.Limit < 0 || params.Limit > 100 {
			return fail("INVALID_PARAMS", errors.New("query/identity/limit exceeds bounds"))
		}
		catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
		if err != nil {
			return fail("CATALOG_UNAVAILABLE", err)
		}
		if request.Method == "catalog.get" {
			if params.ID == "" || params.Query != "" || params.Family != "" || params.Limit != 0 {
				return fail("INVALID_PARAMS", errors.New("catalog.get requires only id"))
			}
			found := false
			for _, r := range catalog.Records {
				if r.ID == params.ID {
					result = entry(r)
					found = true
					break
				}
			}
			if !found {
				return fail("NOT_FOUND", errors.New("catalog ID not found"))
			}
		} else {
			if params.ID != "" {
				return fail("INVALID_PARAMS", errors.New("catalog.search does not accept id"))
			}
			if params.Limit == 0 {
				params.Limit = 20
			}
			q := strings.ToLower(strings.TrimSpace(params.Query))
			records := append([]components.ComponentRecord(nil), catalog.Records...)
			sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
			items := []CatalogEntry{}
			total := 0
			for _, r := range records {
				if params.Family != "" && r.Family != params.Family {
					continue
				}
				if q != "" && !strings.Contains(strings.ToLower(r.ID+" "+r.Name+" "+r.MPN), q) {
					continue
				}
				total++
				if len(items) < params.Limit {
					items = append(items, entry(r))
				}
			}
			result = struct {
				Items     []CatalogEntry `json:"items"`
				Total     int            `json:"total"`
				Truncated bool           `json:"truncated"`
			}{items, total, total > len(items)}
		}
	case "pinmap.get":
		var params struct {
			Symbol    string `json:"symbol"`
			Footprint string `json:"footprint"`
		}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		if params.Symbol == "" || params.Footprint == "" || len(params.Symbol) > 256 || len(params.Footprint) > 256 {
			return fail("INVALID_PARAMS", errors.New("exact symbol and footprint IDs required"))
		}
		found := false
		for _, m := range pinmap.Builtins() {
			if m.Symbol == params.Symbol && m.Footprint == params.Footprint {
				result = m
				found = true
				break
			}
		}
		if !found {
			return fail("NOT_FOUND", errors.New("no registered pin map; absence is not approval"))
		}
	case "schematic.inspect":
		var params struct {
			Path string `json:"path"`
		}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		if len(params.Path) == 0 || len(params.Path) > 4096 {
			return fail("INVALID_PARAMS", errors.New("bounded local path required"))
		}
		report, _, err := schematicreview.Analyze(ctx, params.Path, schematicreview.Options{})
		if err != nil {
			return fail("INSPECTION_FAILED", err)
		}
		result = report
	case "evidence.verify":
		var params struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		if params.Path == "" || len(params.Path) > 4096 {
			return fail("INVALID_PARAMS", errors.New("bounded local evidence path required"))
		}
		var err error
		switch params.Kind {
		case "schematic":
			err = schematicassistant.Verify(params.Path)
		case "review":
			err = schematicreview.Verify(params.Path)
		case "conformance":
			_, err = numericalconformance.Inspect(params.Path)
		case "reproduction":
			_, _, err = reproduction.Inspect(params.Path)
		case "component-test":
			err = componentonboarding.VerifySelfTest(params.Path)
		default:
			return fail("INVALID_PARAMS", errors.New("known evidence kind required"))
		}
		if err != nil {
			return fail("INTEGRITY_FAILED", err)
		}
		result = map[string]any{"integrity_verified": true, "independent_attestation": false, "checks_rerun": false}
	case "family.check":
		var params struct {
			Configuration boardfamily.Config `json:"configuration"`
		}
		if err := decode(&params); err != nil {
			return fail("INVALID_PARAMS", err)
		}
		electrical, err := boardfamily.Check(params.Configuration)
		if err != nil {
			return fail("UNSUPPORTED_CONFIGURATION", err)
		}
		result = map[string]any{"electrical": electrical, "native_checks_run": false, "hardware_validated": false}
	default:
		return fail("UNSUPPORTED_METHOD", errors.New("method is not supported by protocol v1"))
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fail("ENCODING_FAILED", err)
	}
	if len(encoded) > MaxResponse-1024 {
		return fail("RESPONSE_TOO_LARGE", errors.New("reduce requested scope; response exceeds 2 MiB"))
	}
	response.OK = true
	response.Result = encoded
	return response
}
