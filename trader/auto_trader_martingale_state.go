package trader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type martingalePersistedState struct {
	AnchorSide       string  `json:"anchor_side"`
	AnchorPrice      float64 `json:"anchor_price"`
	DayStartEquity   float64 `json:"day_start_equity"`
	DayResetUnix     int64   `json:"day_reset_unix"`
	DailyPaused      bool    `json:"daily_paused"`
}

func martingaleStateDir() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/data.db"
	}
	return filepath.Join(filepath.Dir(dbPath), "martingale_state")
}

func (at *AutoTrader) martingaleStatePath() string {
	return filepath.Join(martingaleStateDir(), at.id+".json")
}

func (at *AutoTrader) martingaleLoadPersistedState() {
	path := at.martingaleStatePath()
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var st martingalePersistedState
	if json.Unmarshal(b, &st) != nil {
		return
	}
	at.martingaleMu.Lock()
	defer at.martingaleMu.Unlock()
	at.martingaleSide = st.AnchorSide
	at.martingaleAnchorPrice = st.AnchorPrice
	at.martingaleDayStartEquity = st.DayStartEquity
	if st.DayResetUnix > 0 {
		at.martingaleDayReset = time.Unix(st.DayResetUnix, 0)
	}
	at.martingaleDailyPaused = st.DailyPaused
}

func (at *AutoTrader) martingaleSavePersistedState() {
	_ = os.MkdirAll(martingaleStateDir(), 0o755)
	at.martingaleMu.Lock()
	st := martingalePersistedState{
		AnchorSide:     at.martingaleSide,
		AnchorPrice:    at.martingaleAnchorPrice,
		DayStartEquity: at.martingaleDayStartEquity,
		DailyPaused:    at.martingaleDailyPaused,
	}
	if !at.martingaleDayReset.IsZero() {
		st.DayResetUnix = at.martingaleDayReset.Unix()
	}
	at.martingaleMu.Unlock()
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(at.martingaleStatePath(), b, 0o644)
}
