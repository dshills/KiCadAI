// Comparison-only normalization. Never serialize this content over raw evidence.
export function normalizeReplay(file, raw) {
  let content = raw.toString('utf8');
  const fields = [];
  const timestamp = '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}';
  const zoned = `${timestamp}[+-][0-9]{2}:[0-9]{2}`;
  function replace(label, expression) {
    const matches = [...content.matchAll(expression)];
    if (matches.length !== 1) throw new Error(`${file}: expected exactly one ${label}`);
    content = content.replace(expression, '$1<NATIVE_TIMESTAMP>$2');
    fields.push(label);
  }
  if (/^manufacturing\/gerbers\/board-[A-Za-z_]+\.gbr$/.test(file)) {
    replace('Gerber X2 creation timestamp', new RegExp(`^(%TF\\.CreationDate,)${zoned}(\\*%)$`, 'gm'));
    replace('Gerber creator comment timestamp', /^(G04 Created by KiCad \(PCBNEW 10\.0\.3\) date )[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}(\*)$/gm);
  } else if (/^manufacturing\/drill\/board-(?:PTH|NPTH)\.drl$/.test(file)) {
    replace('Excellon creator comment timestamp', new RegExp(`^(; DRILL file KiCad 10\\.0\\.3 date )${timestamp}()$`, 'gm'));
    replace('Excellon X2 creation timestamp', new RegExp(`^(; #@! TF\\.CreationDate,)${zoned}()$`, 'gm'));
  } else if (file === 'manufacturing/gerbers/board-job.gbrjob') {
    const date = JSON.parse(content)?.Header?.CreationDate;
    if (typeof date !== 'string' || !new RegExp(`^${zoned}$`).test(date)) throw new Error('missing job header timestamp');
    replace('Gerber job header creation timestamp', new RegExp(`^(    "CreationDate": ")${zoned}("[,]?)$`, 'gm'));
  } else if (file === 'manufacturing/drill-report.txt') {
    replace('Drill report creation timestamp', new RegExp(`^(Created on )${timestamp}()$`, 'gm'));
  } else if (/^(?:preview\/(?:board|pcb)|manufacturing\/drill\/board-(?:PTH|NPTH)-drl_map)\.svg$/.test(file)) {
    replace('Native SVG title timestamp', new RegExp(`^(<title>SVG Image created as [A-Za-z0-9_.-]+ date )${timestamp}( </title>)$`, 'gm'));
  }
  return { content, normalized_fields: fields };
}
