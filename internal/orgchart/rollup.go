package orgchart

import "time"

type RollupSummary struct {
	DirectReports int
	ActiveCount   int
	LastActivity  time.Time
}

type ActivityInfo struct {
	Status     Status
	LastSeenAt time.Time
}

func Rollup(g *Graph, workerID string) (RollupSummary, error) {
	return RollupWithActivity(g, workerID, nil)
}

func RollupWithActivity(g *Graph, workerID string, activity map[string]ActivityInfo) (RollupSummary, error) {
	children, err := g.Children(workerID)
	if err != nil {
		return RollupSummary{}, err
	}

	sum := RollupSummary{DirectReports: len(children)}
	for _, c := range children {
		status, lastSeen := c.Status, c.LastSeenAt
		if a, ok := activity[c.ID]; ok {
			status, lastSeen = a.Status, a.LastSeenAt
		}
		if status == StatusConnected {
			sum.ActiveCount++
		}
		if lastSeen.After(sum.LastActivity) {
			sum.LastActivity = lastSeen
		}
	}
	return sum, nil
}
