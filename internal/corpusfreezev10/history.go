package corpusfreezev10

import "kicadai/internal/corpushistoryv9"

const PredecessorHistoricalCommitmentsSHA256 = corpushistoryv9.PredecessorHistoricalCommitmentsSHA256

const (
	HistoricalRawCount        = corpushistoryv9.HistoricalRawCount
	HistoricalNeutralCount    = corpushistoryv9.HistoricalNeutralCount
	HistoricalNormalizedCount = corpushistoryv9.HistoricalNormalizedCount
)

type HistoricalCommitments = corpushistoryv9.HistoricalCommitments
type CommitmentEntry = corpushistoryv9.CommitmentEntry

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	return corpushistoryv9.LoadHistoricalCommitments(path)
}

func ValidateHistoricalBoundary(value HistoricalCommitments) error {
	return corpushistoryv9.ValidateHistoricalBoundary(value)
}

func ExtendHistoricalCommitments(previous []byte, entries []CommitmentEntry) ([]byte, error) {
	return corpushistoryv9.ExtendHistoricalCommitments(previous, entries)
}
