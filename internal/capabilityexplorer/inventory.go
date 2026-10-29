// Package capabilityexplorer is the supported user-workflow inventory. It
// delegates graph/model contracts to their existing authoritative registries.
package capabilityexplorer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"kicadai/internal/generationcapability"
	"kicadai/internal/numericalconformance"
	"kicadai/internal/schematicassistant"
	"kicadai/internal/simmodel"
	"sort"
)

type Entry struct {
	ID            string   `json:"id"`
	Summary       string   `json:"summary"`
	InputContract string   `json:"input_contract"`
	Limits        []string `json:"limits"`
	Tools         []string `json:"tools"`
	Commands      []string `json:"commands"`
	EvidenceLevel string   `json:"evidence_level"`
	Evidence      string   `json:"evidence"`
	Limitations   []string `json:"limitations"`
}
type Inventory struct {
	Schema         string  `json:"schema"`
	Scope          string  `json:"scope"`
	RegistrySHA256 string  `json:"registry_sha256"`
	Entries        []Entry `json:"entries"`
}

func Build() Inventory {
	entries := []Entry{}
	for _, family := range schematicassistant.Families() {
		entries = append(entries, Entry{ID: family, Summary: "Confirmed, offline " + family + " schematic and explanatory report", InputContract: schematicassistant.Schema, Limits: schematicassistant.SupportedLimits(family), Tools: []string{"none for internal checks; installed kicad-cli for opt-in ERC/preview"}, Commands: []string{"kicadai --output ./out/" + family + " schematic example " + family, "kicadai --output ./out/session schematic new"}, EvidenceLevel: "analytical_and_internal_checks; native_erc_opt_in", Evidence: "specs/schematic-first-workflow/acceptance.json", Limitations: []string{"Ideal passives and zero-impedance source; no physical validation or PCB output", "Confirmation hash detects changed specs; it is not engineering approval"}})
	}
	entries = append(entries, productEntries()...)
	entries = append(entries,
		Entry{ID: "numerical-conformance", Summary: "Compare production Go MNA, independent equations, and optional ngspice", InputContract: numericalconformance.Schema, Limits: []string{fmt.Sprintf("%d frozen cases; 3-minute total / 15-second per external case; no retries", len(numericalconformance.Fixtures())), "DC/AC: 1e-8 V absolute + 1e-7 relative; transient: 1e-8 V + 0.5% relative"}, Tools: []string{"none for analytical comparison", "ngspice exactly " + numericalconformance.NgspiceVersion + " for external conformance"}, Commands: []string{"kicadai --output ./out/conformance conformance run", "kicadai --ngspice /path/to/ngspice-47 --output ./out/external conformance run"}, EvidenceLevel: "per_run_result_required", Evidence: "report.json in each conformance bundle", Limitations: []string{"Only ideal passive DC, AC and RC transient; no general solver certification", "Without --ngspice the external status is not_checked, never conformant"}},
		Entry{ID: "reproduction", Summary: "Capture exact schematic input and first failing stage; replay offline", InputContract: "kicadai.reproduction.v1", Limits: []string{"64 KiB session input; one run; no retries or automated minimization", "Evidence: 256 files / 2 MiB each / 32 MiB total maximum"}, Tools: []string{"none; native checks must be explicitly requested again"}, Commands: []string{"kicadai --request ./session.json --output ./out/repro repro schematic", "kicadai --target ./out/repro --output ./out/replayed repro replay"}, EvidenceLevel: "integrity_checked_not_attested", Evidence: "reproduction.json and manifest.json", Limitations: []string{"Only the confirmed schematic lane; conformance failures have their own replay command", "Never runs a command, path or SPICE input supplied by an untrusted package"}},
		Entry{ID: "guided-clarification", Summary: "Collect compiler-owned questions and bind answers to prior evidence", InputContract: "existing behavioralintent follow-up contract", Limits: []string{"1–32 questions; bounded answers; prior source/proposal/capability hashes must match"}, Tools: []string{"none for answer collection; recorded provider optional"}, Commands: []string{"kicadai --target ./prior-compilation --output ./out/answers intent clarify"}, EvidenceLevel: "hash_bound_compilation_not_electrical_validation", Evidence: "docs/schematic-assistant.md", Limitations: []string{"Does not automatically call a live provider or make unsupported designs supported"}})
	for _, profile := range generationcapability.All() {
		entries = append(entries, Entry{ID: "generation/" + profile.ProfileID, Summary: "Existing " + string(profile.Kind) + " generation contract", InputContract: profile.InputContract, Limits: profile.Supports, Tools: []string{"catalog resolution; native KiCad for native acceptance; provider only when explicitly selected"}, Commands: []string{"kicadai capability generation --json"}, EvidenceLevel: "declared_contract_requires_per_run_acceptance", Evidence: "docs/ai-generation.md", Limitations: append(append([]string{}, profile.Limitations...), profile.RequiredEvidence...)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return Inventory{Schema: "kicadai.capability-explorer.v1", Scope: "Supported front-door workflows and existing generation profiles; not every experimental command, not installed-tool detection, not hardware approval", RegistrySHA256: simmodel.RegistryHash(), Entries: entries}
}
func Markdown() []byte {
	var out bytes.Buffer
	inventory := Build()
	out.WriteString("# Capability explorer\n\nGenerated by `kicadai capability docs`. Do not edit the inventory by hand.\n\n" + inventory.Scope + ".\n\n")
	for _, entry := range inventory.Entries {
		fmt.Fprintf(&out, "## %s\n\n%s.\n\nEvidence level: `%s`. Evidence: `%s`.\n\nInput contract: `%s`.\n\n", entry.ID, entry.Summary, entry.EvidenceLevel, entry.Evidence, entry.InputContract)
		for _, group := range []struct {
			title  string
			values []string
		}{{"Supported bounds", entry.Limits}, {"Required tools", entry.Tools}, {"Limits and acceptance requirements", entry.Limitations}} {
			fmt.Fprintf(&out, "%s:\n\n", group.title)
			for _, line := range group.values {
				fmt.Fprintf(&out, "- %s\n", line)
			}
			out.WriteByte('\n')
		}
		out.WriteString("```sh\n")
		for _, command := range entry.Commands {
			out.WriteString(command + "\n")
		}
		out.WriteString("```\n\n")
	}
	return append(bytes.TrimRight(out.Bytes(), "\n"), '\n')
}
func Lookup(id string) (Entry, bool) {
	for _, entry := range Build().Entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}
func CanonicalJSON() ([]byte, error) { return json.MarshalIndent(Build(), "", "  ") }
