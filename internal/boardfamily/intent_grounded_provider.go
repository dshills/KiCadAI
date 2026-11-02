package boardfamily

import (
	"context"
	"net/http"
)

func InterpretGroundedWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, groundedProtocol)
}

func InspectGroundedJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, groundedProtocol)
}
