package output

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"tsastat/internal/model"
)

type FilterSortOptions struct {
	TID      int
	Comm     string
	ShowIdle bool
	Sort     string
}

func FilterAndSort(stats []model.ThreadIntervalStats, opts FilterSortOptions) ([]model.ThreadIntervalStats, error) {
	filtered := make([]model.ThreadIntervalStats, 0, len(stats))
	for _, stat := range stats {
		if opts.TID > 0 && stat.TID != opts.TID {
			continue
		}
		if opts.Comm != "" && !matchComm(stat.Comm, opts.Comm) {
			continue
		}
		if !opts.ShowIdle && stat.TotalObserved > 0 && stat.Duration(model.StateIdle) == stat.TotalObserved {
			continue
		}
		filtered = append(filtered, stat)
	}

	field := opts.Sort
	if field == "" {
		field = "tid"
	}

	switch field {
	case "tid":
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].TID < filtered[j].TID
		})
	case "comm":
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Comm == filtered[j].Comm {
				return filtered[i].TID < filtered[j].TID
			}
			return filtered[i].Comm < filtered[j].Comm
		})
	case "running":
		sortByDuration(filtered, model.StateRunning)
	case "sleeping":
		sortByDuration(filtered, model.StateSleeping)
	case "uninterruptible":
		sortByDuration(filtered, model.StateUninterruptible)
	case "total":
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].TotalObserved == filtered[j].TotalObserved {
				return filtered[i].TID < filtered[j].TID
			}
			return filtered[i].TotalObserved > filtered[j].TotalObserved
		})
	default:
		return nil, fmt.Errorf("unsupported sort field %q", opts.Sort)
	}

	return filtered, nil
}

func matchComm(comm, pattern string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		matched, err := path.Match(pattern, comm)
		if err == nil {
			return matched
		}
	}
	return strings.Contains(comm, pattern)
}

func sortByDuration(stats []model.ThreadIntervalStats, state model.ThreadState) {
	sort.SliceStable(stats, func(i, j int) bool {
		left := stats[i].Duration(state)
		right := stats[j].Duration(state)
		if left == right {
			return stats[i].TID < stats[j].TID
		}
		return left > right
	})
}
