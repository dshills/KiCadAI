# Stop 01 and approved continuation

The first process exited 1 after useful-02. Its response is accounted and preserved; the model used a non-verbatim `Humidity ... not needed` quote. This is a structured-extraction validation failure, not authentication, quota or transport failure. The application generated no board for that case. Do not retry it or repair the response.

Before continuation, the read-only audit passed all integrity/accounting checks: two attempts, two completed responses, 100000 micro-USD reserved and 2422 micro-USD estimated usage. The runner recorded terminal children, and `ps -p 59748,59761,59811` returned no processes. The checkpoint state and ledger are byte-identical copies made before any continuation.

APPROVAL-02.json already authorizes resuming only never-attempted cases. Resume starts at useful-03; no model, code, contract, case, ledger prefix, limits or previous output changes are permitted. Twelve physical slots remain under the same 14-request / USD 1 cap.
