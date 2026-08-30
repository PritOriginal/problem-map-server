package usecase

import "time"

// SetNow replaces the clock of the settings cache (tests only).
func (uc *Settings) SetNow(now func() time.Time) { uc.now = now }
