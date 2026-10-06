// SPDX-License-Identifier: GPL-2.0-or-later
package migrate

import "time"

// A bounded crash budget prevents a broken configuration from becoming a hot
// restart loop. The enclosing manager owns both the process and the data lock.
type recoveryBudget struct {
	attempts int
	window   time.Time
	next     time.Time
}

func (b *recoveryBudget) allow(now time.Time) bool {
	if b.window.IsZero() || now.Sub(b.window) >= 5*time.Minute {
		b.window = now
		b.attempts = 0
		b.next = now
	}
	if b.attempts >= 3 || now.Before(b.next) {
		return false
	}
	b.attempts++
	b.next = now.Add([]time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}[b.attempts-1])
	return true
}
