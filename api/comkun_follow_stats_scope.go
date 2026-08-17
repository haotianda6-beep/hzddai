package api

import "nofx/store"

func scopeComkunFollowingStats(rows []store.ComkunFollowingStatsRow, mapping map[string]string, userID string, owned map[string]struct{}) []store.ComkunFollowingStatsRow {
	scoped := make([]store.ComkunFollowingStatsRow, 0, len(rows))
	for _, row := range rows {
		if mapping[row.SourceStrategyID] == userID {
			scoped = append(scoped, row)
			continue
		}
		if _, ok := owned[row.StrategyID]; ok {
			scoped = append(scoped, row)
		}
	}
	return scoped
}
