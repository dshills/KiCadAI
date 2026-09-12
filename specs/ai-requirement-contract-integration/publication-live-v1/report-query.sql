-- Read-only SQLite JSON aggregation of authenticated receipts and clause audits.
-- Run from the repository root: sqlite3 -json :memory: ".read specs/ai-requirement-contract-integration/publication-live-v1/report-query.sql"
WITH receipts AS (
 SELECT json_extract(value,'$.case_id') case_id,
        COUNT(*) requests,
        SUM(json_extract(value,'$.estimated_or_reserved_microusd')) cost_microusd
 FROM json_each(readfile('specs/ai-requirement-contract-integration/publication-live-v1/authentication.json'),'$.requests')
 GROUP BY case_id
), reviewed AS (
 SELECT CAST(c.key AS INTEGER)+1 sequence, c.value audit,
        json_extract(c.value,'$.case_id') case_id,
        (SELECT COUNT(*) FROM json_each(c.value,'$.clauses') AS clause WHERE json_extract(clause.value,'$.status')='pass') passing_clauses,
        (SELECT COUNT(*) FROM json_each(c.value,'$.clauses')) all_clauses
 FROM json_each(readfile('specs/ai-requirement-contract-integration/publication-live-v1/audits.json'),'$.cases') AS c
)
SELECT sequence AS "order", reviewed.case_id,
 json_extract(audit,'$.title') title, json_extract(audit,'$.kind') kind,
 receipts.requests, json_extract(audit,'$.initial_attempts') initial_requests,
 json_extract(audit,'$.follow_up_attempts') follow_up_requests,
 json_extract(audit,'$.first_attempt_status') first_status,
 COALESCE(NULLIF(json_extract(audit,'$.follow_up_final_status'),''),json_extract(audit,'$.initial_final_status')) final_status,
 CASE reviewed.case_id
 WHEN 'I01' THEN 'Faithful after correction'
 WHEN 'I02' THEN 'Failed: coverage reference'
 WHEN 'I03' THEN 'Failed: coverage/endpoints'
 WHEN 'I04' THEN 'Failed: external output source'
 WHEN 'R01' THEN 'Valid refusal'
 WHEN 'R02' THEN 'Valid refusal'
 WHEN 'C01' THEN 'Faithful clarification'
 WHEN 'C02' THEN 'Failed: follow-up contract' END outcome,
 passing_clauses || '/' || all_clauses clauses,
 cost_microusd/1000000.0 estimated_usd,
 (passing_clauses=all_clauses AND CASE json_extract(audit,'$.kind')
 WHEN 'ready' THEN json_extract(audit,'$.initial_final_status')='ready'
 WHEN 'refusal' THEN json_extract(audit,'$.initial_final_status')='unsupported'
 ELSE json_extract(audit,'$.initial_final_status')='needs_clarification' AND json_extract(audit,'$.follow_up_final_status')='ready' END) faithful,
 0 sealed_replay_passed
FROM reviewed JOIN receipts USING (case_id)
ORDER BY sequence;
