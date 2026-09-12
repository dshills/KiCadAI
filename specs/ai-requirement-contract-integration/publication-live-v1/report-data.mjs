import {derive} from './derive.mjs';
const outcomes={"I01":"Faithful after correction","I02":"Failed: coverage reference","I03":"Failed: coverage/endpoints","I04":"Failed: external output source","R01":"Valid refusal","R02":"Valid refusal","C01":"Faithful clarification","C02":"Failed: follow-up contract"};
const {summary}=derive();
const rows=summary.cases.map((c,i)=>({order:i+1,case_id:c.case_id,title:c.title,kind:c.kind,requests:c.requests,initial_requests:c.initial_attempts,follow_up_requests:c.follow_up_attempts,first_status:c.first_attempt_status,final_status:c.follow_up_final_status||c.initial_final_status,outcome:outcomes[c.case_id],clauses:c.clause_passes+'/'+c.clause_count,estimated_usd:c.estimated_or_reserved_usd,faithful:c.observed_faithful_workflow,sealed_replay_passed:false}));
console.log(JSON.stringify(rows,null,2));
