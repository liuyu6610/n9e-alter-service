package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Field struct {
	all bool
	set map[int]struct{}
}

type Schedule struct {
	Minute Field
	Hour   Field
	Dom    Field
	Month  Field
	Dow    Field
}

func Parse(expr string) (Schedule, error) {
	expr = strings.TrimSpace(expr)
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("cron expr must have 5 fields")
	}

	min, err := parseField(parts[0], 0, 59, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseField(parts[2], 1, 31, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("dom: %w", err)
	}
	month, err := parseField(parts[3], 1, 12, false)
	if err != nil {
		return Schedule{}, fmt.Errorf("month: %w", err)
	}
	dow, err := parseField(parts[4], 0, 7, true)
	if err != nil {
		return Schedule{}, fmt.Errorf("dow: %w", err)
	}

	return Schedule{Minute: min, Hour: hour, Dom: dom, Month: month, Dow: dow}, nil
}

func (s Schedule) Next(after time.Time) time.Time {
	start := after.Truncate(time.Minute).Add(time.Minute)
	cur := start
	max := 366 * 24 * 60
	for i := 0; i < max; i++ {
		if s.Matches(cur) {
			return cur
		}
		cur = cur.Add(time.Minute)
	}
	return time.Time{}
}

func (s Schedule) Matches(t time.Time) bool {
	m := t.Minute()
	h := t.Hour()
	d := t.Day()
	mo := int(t.Month())
	w := int(t.Weekday())

	if !s.Minute.contains(m) {
		return false
	}
	if !s.Hour.contains(h) {
		return false
	}
	if !s.Dom.contains(d) {
		return false
	}
	if !s.Month.contains(mo) {
		return false
	}
	if !s.Dow.contains(w) {
		return false
	}
	return true
}

func (f Field) contains(v int) bool {
	if f.all {
		return true
	}
	_, ok := f.set[v]
	return ok
}

func parseField(s string, min int, max int, dow bool) (Field, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Field{}, fmt.Errorf("empty")
	}
	if s == "*" {
		return Field{all: true}, nil
	}

	set := map[int]struct{}{}
	segs := strings.Split(s, ",")
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if err := addSegment(set, seg, min, max, dow); err != nil {
			return Field{}, err
		}
	}
	if len(set) == 0 {
		return Field{}, fmt.Errorf("no values")
	}
	return Field{set: set}, nil
}

func addSegment(set map[int]struct{}, seg string, min int, max int, dow bool) error {
	step := 1
	if strings.Contains(seg, "/") {
		parts := strings.SplitN(seg, "/", 2)
		seg = strings.TrimSpace(parts[0])
		st := strings.TrimSpace(parts[1])
		n, err := strconv.Atoi(st)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid step")
		}
		step = n
	}

	if seg == "*" {
		for v := min; v <= max; v += step {
			vv := v
			if dow && vv == 7 {
				vv = 0
			}
			set[vv] = struct{}{}
		}
		return nil
	}

	if strings.Contains(seg, "-") {
		parts := strings.SplitN(seg, "-", 2)
		lo, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid range")
		}
		if lo > hi {
			return fmt.Errorf("invalid range")
		}
		if lo < min || hi > max {
			return fmt.Errorf("range out of bounds")
		}
		for v := lo; v <= hi; v += step {
			vv := v
			if dow && vv == 7 {
				vv = 0
			}
			set[vv] = struct{}{}
		}
		return nil
	}

	n, err := strconv.Atoi(seg)
	if err != nil {
		return fmt.Errorf("invalid number")
	}
	if n < min || n > max {
		return fmt.Errorf("number out of bounds")
	}
	if dow && n == 7 {
		n = 0
	}
	set[n] = struct{}{}
	return nil
}
