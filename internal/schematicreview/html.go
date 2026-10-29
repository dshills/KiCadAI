package schematicreview

import (
	"bytes"
	"html/template"
)

func render(report Report) ([]byte, error) {
	page, err := template.New("review").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>Schematic review</title><style>body{font:16px system-ui;max-width:1100px;margin:2rem auto;padding:0 1rem;color:#192331;background:#f5f7fa}table{border-collapse:collapse;width:100%;background:white}td,th{text-align:left;border:1px solid #b8c3cf;padding:.5rem;overflow-wrap:anywhere}section{margin:2rem 0}li{margin:.6rem 0}</style><h1>Schematic review</h1><p>Root: {{.RootSheet}}. Result: {{.Status}}. Source unchanged: {{.SourceUnchanged}}. Native ERC: {{.NativeERC}}.</p><p>This review explains supported checks. It does not approve the circuit or predict its physical performance.</p>{{range .Sheets}}<section><h2>{{.Path}}</h2><p>{{.Inspection.SymbolCount}} symbols, {{.Inspection.WireCount}} wires, {{.Inspection.LabelCount}} labels, {{.Inspection.SheetCount}} child sheets. SHA256: {{.SHA256}}.</p><h3>Components</h3><table><tr><th>Reference</th><th>Value and library</th><th>Possible purpose</th></tr>{{range .Parts}}<tr><td>{{.Reference}}</td><td>{{.Value}} — {{.Library}}</td><td>{{.Purpose}}</td></tr>{{end}}</table><h3>Supported checks</h3><ul>{{range .Evaluation.Checks}}<li>{{.Name}}: {{.Status}}{{range .Issues}} — {{.Message}} {{.Suggestion}}{{end}}</li>{{end}}</ul><h3>Unsupported or preservation only</h3><ul>{{range .Inspection.Unsupported}}<li>{{.Kind}}: {{.Count}} unsupported objects</li>{{end}}{{range .Inspection.PreservationOnly}}<li>{{.Kind}}: {{.Count}} preserved but not fully assessed objects</li>{{end}}</ul></section>{{end}}<h2>Limits</h2><ul>{{range .Limitations}}<li>{{.}}</li>{{end}}</ul><p><a href="report.json">Machine-readable report</a> · <a href="manifest.json">Integrity inventory</a></p></html>`)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = page.Execute(&out, report)
	return out.Bytes(), err
}
