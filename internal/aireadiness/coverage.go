package aireadiness

import "sort"

type DomainCoverage struct {
	Domain      string              `json:"domain"`
	Total       int                 `json:"total"`
	ByCategory  []CategoryCoverage  `json:"by_category"`
	ByReadiness []ReadinessCoverage `json:"by_readiness"`
	NextTasks   []TaskCoverage      `json:"next_tasks"`
}

type CategoryCoverage struct {
	Category Category `json:"category"`
	Count    int      `json:"count"`
}

type ReadinessCoverage struct {
	Readiness Readiness `json:"readiness"`
	Count     int       `json:"count"`
}

type TaskCoverage struct {
	Task  TaskType `json:"task"`
	Count int      `json:"count"`
}

func SummarizeDomain(matrix Matrix, domain string) DomainCoverage {
	categoryCounts := map[Category]int{}
	readinessCounts := map[Readiness]int{}
	taskCounts := map[TaskType]int{}
	summary := DomainCoverage{Domain: domain}
	for _, record := range matrix.Records {
		if record.Domain != domain {
			continue
		}
		summary.Total++
		categoryCounts[record.Category]++
		readinessCounts[record.Readiness]++
		taskCounts[record.NextTask]++
	}
	summary.ByCategory = categoryCoverageList(categoryCounts)
	summary.ByReadiness = readinessCoverageList(readinessCounts)
	summary.NextTasks = taskCoverageList(taskCounts)
	return summary
}

func categoryCoverageList(counts map[Category]int) []CategoryCoverage {
	items := make([]CategoryCoverage, 0, len(counts))
	for category, count := range counts {
		items = append(items, CategoryCoverage{Category: category, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Category < items[j].Category
	})
	return items
}

func readinessCoverageList(counts map[Readiness]int) []ReadinessCoverage {
	items := make([]ReadinessCoverage, 0, len(counts))
	for readiness, count := range counts {
		items = append(items, ReadinessCoverage{Readiness: readiness, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return readinessRank(items[i].Readiness) < readinessRank(items[j].Readiness)
	})
	return items
}

func taskCoverageList(counts map[TaskType]int) []TaskCoverage {
	items := make([]TaskCoverage, 0, len(counts))
	for task, count := range counts {
		items = append(items, TaskCoverage{Task: task, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Task < items[j].Task
	})
	return items
}

func readinessRank(readiness Readiness) int {
	switch readiness {
	case ReadinessMissing:
		return 0
	case ReadinessDraft:
		return 1
	case ReadinessConnectivity:
		return 2
	case ReadinessCandidate:
		return 3
	case ReadinessVerified:
		return 4
	default:
		return 100
	}
}
