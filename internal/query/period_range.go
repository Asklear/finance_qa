package query

import (
	"fmt"
	"time"
)

func periodsBetween(from, to string) ([]string, error) {
	start, err := time.Parse("2006-01", from)
	if err != nil {
		return nil, fmt.Errorf("parse start period %s: %w", from, err)
	}
	end, err := time.Parse("2006-01", to)
	if err != nil {
		return nil, fmt.Errorf("parse end period %s: %w", to, err)
	}
	if start.After(end) {
		return nil, fmt.Errorf("invalid period range %s~%s", from, to)
	}
	periods := make([]string, 0, 12)
	for current := start; !current.After(end); current = current.AddDate(0, 1, 0) {
		periods = append(periods, current.Format("2006-01"))
	}
	return periods, nil
}
