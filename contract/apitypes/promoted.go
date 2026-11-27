package apitypes

import "time"

// What a Promoted app adds beyond its resources (CONTROL-PLANE.md §6.18): hourly backup points.

// BackupPoint is one hourly backup point: when, at which position in the database's history, and
// whether the node saw it reach the backup archive.
type BackupPoint struct {
	At       time.Time `json:"at"`
	LSN      string    `json:"lsn"`
	Archived bool      `json:"archived"`
}

// BackupPoints is GET /orgs/:org/apps/:app/backup-points: whether points are being taken now (a
// Promoted app in force), how many days back a restore reaches, and the points of the last 30
// days, newest first.
type BackupPoints struct {
	Hourly     bool          `json:"hourly"`
	WindowDays int64         `json:"window_days"`
	Points     []BackupPoint `json:"points"`
}
