package manifest

import (
	"strconv"
	"strings"
	"time"
)

// cronSets is a parsed five-field expression: one membership set per field. Day of week 7 is
// folded to 0. domStar and dowStar record whether the day fields were "*", because standard
// cron matches a day when either restricted field matches, and both when neither is restricted.
type cronSets struct {
	minute, hour, dom, month, dow [64]bool
	domStar, dowStar              bool
}

func parseCronSets(expr string) (cronSets, bool) {
	if CheckCron(expr) != nil {
		return cronSets{}, false
	}
	fields := strings.Fields(expr)
	var s cronSets
	s.minute = fieldSet(cronFields[0], fields[0])
	s.hour = fieldSet(cronFields[1], fields[1])
	s.dom = fieldSet(cronFields[2], fields[2])
	s.month = fieldSet(cronFields[3], fields[3])
	s.dow = fieldSet(cronFields[4], fields[4])
	if s.dow[7] {
		s.dow[0] = true
	}
	s.domStar = fields[2] == "*"
	s.dowStar = fields[4] == "*"
	return s, true
}

func fieldSet(spec cronField, field string) [64]bool {
	var set [64]bool
	for _, part := range strings.Split(field, ",") {
		rangePart, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			step, _ = strconv.Atoi(stepText)
			// A step past the field's range fires once; capping it keeps v from overflowing.
			step = min(step, spec.max+1)
		}
		lo, hi := spec.min, spec.max
		if rangePart != "*" {
			a, b, isRange := strings.Cut(rangePart, "-")
			lo, _ = strconv.Atoi(a)
			hi = lo
			if isRange {
				hi, _ = strconv.Atoi(b)
			} else if hasStep {
				hi = spec.max
			}
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	return set
}

func (s cronSets) dayMatches(t time.Time) bool {
	dom := s.dom[t.Day()]
	dow := s.dow[int(t.Weekday())]
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dow
	case s.dowStar:
		return dom
	default:
		return dom || dow
	}
}

// NextCron returns the first time strictly after `after` at which the expression fires in the
// given location, and false when the expression is invalid or nothing fires within five years
// (a day-of-month and month pair that never exists, for example).
func NextCron(expr string, loc *time.Location, after time.Time) (time.Time, bool) {
	sets, ok := parseCronSets(expr)
	if !ok {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.UTC
	}
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		switch {
		case !sets.month[int(t.Month())]:
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
		case !sets.dayMatches(t):
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
		case !sets.hour[t.Hour()]:
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
		case !sets.minute[t.Minute()]:
			t = t.Add(time.Minute)
		default:
			return t, true
		}
	}
	return time.Time{}, false
}

// SpacedCron is the expression run at most once every minMinutes (1 to 60): of the minutes the
// minute field names, it keeps the first and then each one at least minMinutes after the last
// kept, and drops any at the end that would come back round too soon before the first of the
// next hour. With 15, "*/5 * * * *" becomes "0,15,30,45 * * * *"; with 60, "*/10 5-21 * * 1-5"
// becomes "0 5-21 * * 1-5". The other fields are untouched. changed is false for an expression
// that is already spaced enough, and for an invalid one, which is returned as it is.
//
// A leading "TZ=<zone>" or "CRON_TZ=<zone>", as the workflow SDKs write a time zone into the
// expression, is kept in front.
func SpacedCron(expr string, minMinutes int) (spaced string, changed bool) {
	if minMinutes <= 1 {
		return expr, false
	}
	if minMinutes > 60 {
		minMinutes = 60
	}
	prefix, rest := "", strings.TrimSpace(expr)
	if first, after, ok := strings.Cut(rest, " "); ok && (strings.HasPrefix(first, "TZ=") || strings.HasPrefix(first, "CRON_TZ=")) {
		prefix, rest = first+" ", strings.TrimSpace(after)
	}
	sets, ok := parseCronSets(rest)
	if !ok {
		return expr, false
	}
	var all, kept []int
	for m := 0; m < 60; m++ {
		if !sets.minute[m] {
			continue
		}
		all = append(all, m)
		if len(kept) == 0 || m-kept[len(kept)-1] >= minMinutes {
			kept = append(kept, m)
		}
	}
	for len(kept) > 1 && kept[0]+60-kept[len(kept)-1] < minMinutes {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == len(all) {
		return expr, false
	}
	minutes := make([]string, len(kept))
	for i, m := range kept {
		minutes[i] = strconv.Itoa(m)
	}
	fields := strings.Fields(rest)
	fields[0] = strings.Join(minutes, ",")
	return prefix + strings.Join(fields, " "), true
}
