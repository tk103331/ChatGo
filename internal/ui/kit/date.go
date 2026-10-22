package kit

import (
	"fmt"
	"time"

	"chatgo/internal/ui/uikit"
)

// DateSeparatorWhen is a short stamp for t: its time today, "昨天", or its
// month and day.
func DateSeparatorWhen(t time.Time, today uikit.Date) string {
	day := uikit.DateOf(t)
	switch day {
	case today:
		return t.Format("15:04")
	case today.AddDays(-1):
		return "昨天"
	}
	return fmt.Sprintf("%d月%d日", int(day.Month), day.Day)
}
