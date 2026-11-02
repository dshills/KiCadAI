import assert from 'node:assert/strict';
import test from 'node:test';
import { normalizeReplay } from './replay-normalization.mjs';

const fixtures = [
  ['manufacturing/gerbers/board-F_Cu.gbr', '%TF.CreationDate,2026-09-13T12:00:00-05:00*%\nG04 Created by KiCad (PCBNEW 10.0.3) date 2026-09-13 12:00:00*\nX12Y34D01*\nM02*\n', 2],
  ['manufacturing/drill/board-PTH.drl', '; DRILL file KiCad 10.0.3 date 2026-09-13T12:00:00\n; #@! TF.CreationDate,2026-09-13T12:00:00-05:00\nX12Y34\nM30\n', 2],
  ['manufacturing/gerbers/board-job.gbrjob', '{\n  "Header": {\n    "CreationDate": "2026-09-13T12:00:00-05:00"\n  },\n  "Size": 12\n}\n', 1],
  ['manufacturing/drill-report.txt', 'Created on 2026-09-13T12:00:00\nT1: 12 holes\n', 1],
  ['preview/pcb.svg', '<title>SVG Image created as pcb.svg date 2026-09-13T12:00:00 </title>\n<path d="M12 34"/>\n', 1],
];
for (const [file, raw, count] of fixtures) {
  test(`${file}: dates only; geometry and metadata remain significant`, () => {
    const initial = normalizeReplay(file, Buffer.from(raw));
    assert.equal(initial.normalized_fields.length, count);
    assert.equal(initial.content, normalizeReplay(file, raw.replaceAll('12:00:00', '19:23:45')).content);
    const changedBody = raw.replaceAll('12 holes', '13 holes').replaceAll('X12', 'X13').replaceAll('"Size": 12', '"Size": 13').replaceAll('M12', 'M13');
    assert.notEqual(initial.content, normalizeReplay(file, changedBody).content);
    assert.throws(() => normalizeReplay(file, raw + raw));
    assert.throws(() => normalizeReplay(file, raw.replaceAll('2026-09-13', 'MISSING')));
  });
}
test('No broad date removal in native, BOM or placement files', () => {
  for (const file of ['board.kicad_pcb', 'configuration.json', 'bom.csv', 'manufacturing/placement.csv']) {
    const content = '2026-09-13T12:00:00 X12Y34\n';
    assert.deepEqual(normalizeReplay(file, content), { content, normalized_fields: [] });
  }
});
