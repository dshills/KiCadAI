package schematicassistant

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
)

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{"number": func(v float64) string { return fmt.Sprintf("%.6g", v) }, "preview": func(data string) template.URL { return template.URL("data:image/svg+xml;base64," + data) }}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<title>{{.Spec.Name}} — KiCadAI schematic review</title>
<style>
:root{color-scheme:light dark;--bg:#f4f6f9;--ink:#182638;--panel:#fff;--line:#dbe1e9;--accent:#175c83}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.6 system-ui,sans-serif}main{max-width:1100px;margin:auto;padding:40px 24px}h1{font-size:clamp(26px,5vw,42px);line-height:1.15;margin:12px 0}h2{font-size:23px}.eyebrow{color:var(--accent);font-weight:700;letter-spacing:.08em;text-transform:uppercase;font-size:12px}.notice{border-left:4px solid #d48c22;padding:14px 20px;background:var(--panel)}section{background:var(--panel);padding:24px;border:1px solid var(--line);border-radius:12px;margin:22px 0}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.metric{border:1px solid var(--line);padding:16px;border-radius:8px}.metric strong{font-size:24px;display:block}.muted,small{color:#52677d}img{width:100%;max-height:640px;object-fit:contain;background:white;border:1px solid var(--line)}table{width:100%;border-collapse:collapse}td,th{text-align:left;vertical-align:top;padding:12px 8px;border-bottom:1px solid var(--line)}th{font-size:13px;text-transform:uppercase}.scroll{overflow-x:auto}a{color:var(--accent)}code{overflow-wrap:anywhere;font-size:13px}.status{font-weight:700;white-space:nowrap}@media(prefers-color-scheme:dark){:root{--bg:#111923;--ink:#e1eaf5;--panel:#1b2836;--line:#344659;--accent:#86c9f3}.muted,small{color:#b3c2d2}}
</style></head><body><main><div class="eyebrow">KiCadAI · offline schematic workflow</div>
<h1>{{.Spec.Name}}</h1><p class="muted">{{.Spec.Family}} · {{.Status}}</p>
<p class="notice"><strong>Schematic candidate—not hardware approval.</strong> Calculations assume the stated ideal circuit. Missing checks are not passes. No PCB or fabrication release is produced.</p>
<section><h2>Your confirmed specification</h2><p>Maximum input: <strong>{{number .Spec.InputV}} V</strong> · R1: {{number .Spec.R1}} Ω · Load: {{number .Spec.Load}} Ω (0 means explicitly unloaded).</p>
{{if .Spec.R2}}<p>R2: {{number .Spec.R2}} Ω</p>{{end}}{{if .Spec.C}}<p>C1: {{number .Spec.C}} F</p>{{end}}
<p><small>Exact specification identity: <code>{{.SpecSHA256}}</code>. This hash binds bytes; it is not an independent approval signature.</small></p></section>
<section><h2>Understand the circuit</h2><img src="{{preview .PreviewData}}" alt="Circuit connection diagram"><p class="muted">{{.DiagramDescription}}</p><p><a href="{{.Spec.Name}}.kicad_pro">Open the native KiCad project</a> · <a href="source-ir.json">Inspect the semantic circuit</a></p>
<div class="scroll"><table><thead><tr><th>Reference</th><th>Value</th><th>Purpose</th></tr></thead><tbody>{{range .Parts}}<tr><td>{{.Ref}}</td><td>{{.Value}}</td><td>{{.Purpose}}</td></tr>{{end}}</tbody></table></div></section>
<section><h2>Calculated behavior</h2><p class="muted">Analytical predictions, not measured voltages or a numerical simulation.</p><div class="grid">{{range .Measurements}}<div class="metric">{{.Name}}<strong>{{number .Value}} {{.Unit}}</strong><small>{{.Explanation}}</small></div>{{end}}</div></section>
<section><h2>What was checked?</h2><div class="scroll"><table><thead><tr><th>Check</th><th>Status</th><th>Meaning / next action</th></tr></thead><tbody>{{range .Checks}}<tr><td>{{.Name}}</td><td class="status">{{.Status}}</td><td>{{.Detail}} {{if .Evidence}}<a href="{{.Evidence}}">Evidence</a>{{end}}</td></tr>{{end}}</tbody></table></div></section>
<section><h2>Limits and next steps</h2><ul>{{range .Limitations}}<li>{{.}}</li>{{end}}</ul><p>Open the schematic in KiCad. To change values, resume the guided session or confirm a new specification, then regenerate into a new directory. Do not repair generated artifacts by hand.</p></section>
<section><h2>Evidence and reproducibility</h2><p><a href="session.json">Confirmed session</a> · <a href="report.json">Machine-readable report</a> · <a href="manifest.json">Complete artifact hashes</a></p><p><code>kicadai --target /path/to/this/bundle schematic verify</code></p><p class="muted">This report contains no external scripts, fonts, trackers, or API calls.</p></section></main></body></html>`))

func writeHTML(root string, report Report) error {
	preview, err := os.ReadFile(filepath.Join(root, report.Diagram))
	if err != nil {
		return err
	}
	report.PreviewData = base64.StdEncoding.EncodeToString(preview)
	file, err := os.Create(filepath.Join(root, "report.html"))
	if err != nil {
		return err
	}
	if err = reportTemplate.Execute(file, report); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
func diagramSVG(s Spec) string {
	shunt := "R2"
	value := 0.0
	if s.R2 != nil {
		value = *s.R2
	} else {
		shunt = "C1"
		value = *s.C
	}
	load := "unloaded"
	if *s.Load > 0 {
		load = fmt.Sprintf("R3 load %.6g ohm", *s.Load)
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 280" role="img" aria-label="Functional passive circuit diagram"><rect width="800" height="280" fill="white"/><g stroke="#24445f" stroke-width="3" fill="none"><path d="M70 90H220M340 90H690M500 90V130M500 200V230M465 230H535"/><rect x="220" y="70" width="120" height="40"/><rect x="480" y="130" width="40" height="70"/></g><g fill="#142d42" font-family="sans-serif" font-size="19"><text x="30" y="60">VIN %.6g V</text><text x="225" y="52">R1 %.6g Ω</text><text x="620" y="60">VOUT</text><text x="545" y="170">%s %.6g %s</text><text x="450" y="265">GND</text><text x="30" y="265">%s</text></g></svg>`, *s.InputV, *s.R1, shunt, value, map[bool]string{true: "Ω", false: "F"}[s.Family == "divider"], load)
}
