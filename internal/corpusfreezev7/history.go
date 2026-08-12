package corpusfreezev7

import "kicadai/internal/corpusfreezev6"

type HistoricalCommitments = corpusfreezev6.HistoricalCommitments

func LoadHistoricalCommitments(path string) (HistoricalCommitments, error) {
	return corpusfreezev6.LoadHistoricalCommitments(path)
}
