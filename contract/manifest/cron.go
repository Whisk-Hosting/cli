package manifest

import (
	"fmt"
	"strconv"
	"strings"
)

type cronField struct {
	name     string
	min, max int
}

var cronFields = []cronField{{"minute", 0, 59}, {"hour", 0, 23}, {"day of month", 1, 31}, {"month", 1, 12}, {"day of week", 0, 7}}

// CheckCron validates a five-field cron expression: each field is *, a number, a range a-b, a
// list of those, or any of them with /step, within the field's range.
func CheckCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron %q must have 5 fields (minute hour day month weekday), has %d", expr, len(fields))
	}
	for i, f := range fields {
		if err := checkCronField(cronFields[i], f); err != nil {
			return fmt.Errorf("cron %q: %s field: %v", expr, cronFields[i].name, err)
		}
	}
	return nil
}

func checkCronField(spec cronField, field string) error {
	for _, part := range strings.Split(field, ",") {
		rangePart, step, hasStep := strings.Cut(part, "/")
		if hasStep {
			n, err := strconv.Atoi(step)
			if err != nil || n < 1 {
				return fmt.Errorf("step %q is not a positive number", step)
			}
		}
		if rangePart == "*" {
			continue
		}
		lo, hi, isRange := strings.Cut(rangePart, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return fmt.Errorf("%q is not a number, range or *", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				return fmt.Errorf("%q is not a valid range", part)
			}
		}
		if a < spec.min || b > spec.max || a > b {
			return fmt.Errorf("%q is outside %d-%d", part, spec.min, spec.max)
		}
	}
	return nil
}
