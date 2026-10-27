package easylist

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

// tasksList has a field of every type an expression can meet.
func tasksList() List {
	return List{
		Name: "tasks", Table: "tasks", Alias: "t", PrimaryKey: "id",
		Fields: []Field{
			{Name: "id", Type: Integer},
			{Name: "qty", Type: Integer},
			{Name: "age", Type: Integer},
			{Name: "score", Type: Float},
			{Name: "price", Type: Numeric},
			{Name: "due", Type: Date},
			{Name: "created_at", Type: Timestamp},
			{Name: "seen_at", Type: Timestamp},
			{Name: "t", Type: Timestamp},
			{Name: "name", Type: Text},
			{Name: "done", Type: Boolean},
			{Name: "ref", Type: UUID},
			{Name: "state", Type: Enum, Values: []string{"open"}},
			{Name: "days", Type: Date, Array: true},
			{Name: "counts", Type: Integer, Array: true},
		},
	}
}

func mustTasks(t *testing.T) *CompiledList {
	t.Helper()
	reg, err := NewRegistry(tasksList())
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("tasks")
	return l
}

// predicates returns the filter's predicates from left to right.
func predicates(e filter.Expr) []filter.Predicate {
	switch n := e.(type) {
	case filter.Predicate:
		return []filter.Predicate{n}
	case filter.Logical:
		return append(predicates(n.Left()), predicates(n.Right())...)
	}
	return nil
}

var (
	utc2 = time.FixedZone("", 2*60*60)
	// tuesday is the reference time of most scenarios: Tuesday 2026-10-27.
	tuesday = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
)

func weekday(d time.Weekday) *time.Weekday { return &d }

func london(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skipf("no time zone database: %v", err)
	}
	return loc
}

// planValue plans input with now and week start and returns the value of
// its only predicate.
func planValue(t *testing.T, input string, now time.Time, ws *time.Weekday) (Value, error) {
	t.Helper()
	expr := parse(t, input)
	p, err := mustTasks(t).Plan(KindPage, Request{Filter: expr, Now: now, WeekStart: ws})
	if err != nil {
		return Value{}, err
	}
	v, ok := p.Value(predicates(expr)[0])
	if !ok {
		t.Fatalf("%s: plan has no value", input)
	}
	return v, nil
}

func date(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func TestExpressionValues(t *testing.T) {
	ldn := london(t)
	cases := []struct {
		name  string
		input string
		now   time.Time
		ws    *time.Weekday
		want  Value
	}{
		// Numbers.
		{"numbers need no reference time", "qty > {2 * 3}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "6"}},
		{"exact decimals", "price > {10 / 4 - 0.1}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "2.4"}},
		{"integer product", "price > {100 * 3}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "300"}},
		{"trailing zeros dropped", "price > {-1.50 * 2}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "-3"}},
		{"fraction kept", "price > {1.10 + 0}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "1.1"}},
		{"zero", "price > {0.5 - 0.5}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "0"}},
		{"negation", "price > {-(2 - 5)}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "3"}},
		{"terminating quotient", "price > {1 / 8}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "0.125"}},
		{"plain notation", "price > {1 / 1024 / 1000000}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "0.0000000009765625"}},
		{"large", "price > {123456789012345678901234567890 * 10}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "1234567890123456789012345678900"}},
		{"not an integer still a number", "qty > {10 / 4}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "2.5"}},
		{"float field", "score > {1 / 4}", time.Time{}, nil, Value{Kind: ValueNumber, Number: "0.25"}},
		{"reference time ignored", "qty > {2 * 3}", tuesday, nil, Value{Kind: ValueNumber, Number: "6"}},

		// Keywords.
		{"now", "t > NOW", tuesday, nil, Value{Kind: ValueTimestamp, Time: tuesday}},
		{"today", "due = TODAY", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 27, time.UTC)}},
		{"time zone decides the date", "due = TODAY", time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC).In(utc2), nil,
			Value{Kind: ValueDate, Time: date(2026, 10, 28, utc2)}},
		{"default week start", "created_at >= START_OF_WEEK", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 26, time.UTC)}},
		{"monday week start", "due >= start_of_week", tuesday, weekday(time.Monday), Value{Kind: ValueDate, Time: date(2026, 10, 26, time.UTC)}},
		{"end of week", "due <= END_OF_WEEK", tuesday, weekday(time.Monday), Value{Kind: ValueDate, Time: date(2026, 11, 1, time.UTC)}},
		{"sunday week start", "due >= START_OF_WEEK", tuesday, weekday(time.Sunday), Value{Kind: ValueDate, Time: date(2026, 10, 25, time.UTC)}},
		{"sunday week end", "due <= END_OF_WEEK", tuesday, weekday(time.Sunday), Value{Kind: ValueDate, Time: date(2026, 10, 31, time.UTC)}},
		{"week starts today", "due >= START_OF_WEEK", tuesday, weekday(time.Tuesday), Value{Kind: ValueDate, Time: date(2026, 10, 27, time.UTC)}},
		{"week starts tomorrow", "due >= START_OF_WEEK", tuesday, weekday(time.Wednesday), Value{Kind: ValueDate, Time: date(2026, 10, 21, time.UTC)}},
		{"start of month", "due >= START_OF_MONTH", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 1, time.UTC)}},
		{"end of month", "due <= END_OF_MONTH", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 31, time.UTC)}},
		{"end of february", "due <= END_OF_MONTH", time.Date(2028, 2, 10, 0, 0, 0, 0, time.UTC), nil, Value{Kind: ValueDate, Time: date(2028, 2, 29, time.UTC)}},
		{"start of year", "due >= START_OF_YEAR", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 1, 1, time.UTC)}},
		{"end of year", "due <= END_OF_YEAR", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 12, 31, time.UTC)}},
		{"date on a timestamp field", "created_at <= END_OF_WEEK", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 1, time.UTC)}},

		// Durations.
		{"date plus days", "due > {TODAY + 5::DAYS}", time.Date(2026, 10, 27, 10, 0, 0, 0, utc2), nil, Value{Kind: ValueDate, Time: date(2026, 11, 1, utc2)}},
		{"date minus days", "due > {TODAY - 30::DAYS}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 9, 27, time.UTC)}},
		{"days first", "due > {5::DAYS + TODAY}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 1, time.UTC)}},
		{"scaled days", "due = {TODAY + 2 * 7::DAYS}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 10, time.UTC)}},
		{"scaled on the right", "due = {TODAY + 7::DAYS * 2}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 10, time.UTC)}},
		{"multiplier expression", "due = {TODAY + (1 + 1) * 7::DAYS}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 10, time.UTC)}},
		{"integral decimal multiplier", "due = {TODAY + 2.0 * 7::DAYS}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 11, 10, time.UTC)}},
		{"negated duration", "due = {TODAY + -(1::DAYS)}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 26, time.UTC)}},
		{"duration minus duration", "due = {TODAY + (3::DAYS - 1::DAYS)}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 29, time.UTC)}},
		{"exact duration division", "t > {NOW + (2::DAYS + 4::HOURS) / 2}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}},
		{"negative divisor", "due = {TODAY + 4::DAYS / -2}", tuesday, nil, Value{Kind: ValueDate, Time: date(2026, 10, 25, time.UTC)}},
		{"minutes", "seen_at > {NOW - 30::MINUTES}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)}},
		{"date plus hours", "t > {TODAY + 9::HOURS}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)}},
		{"mixed units", "t > {NOW + (1::DAYS + 2::HOURS)}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}},
		{"mixed units written out", "t > {NOW + 1::DAYS + 2::HOURS}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}},
		{"days on a timestamp keep the clock", "t > {NOW - 1::DAYS}", tuesday, nil, Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)}},
		{"limit amount", "t > {TODAY - 10000000::MINUTES}", tuesday, nil, Value{Kind: ValueTimestamp, Time: tuesday.Truncate(24 * time.Hour).Add(-10000000 * time.Minute)}},

		// Daylight saving time in Europe/London, where clocks go back on 2026-10-25.
		{"calendar days across a DST change", "t > {NOW + 1::DAYS}", time.Date(2026, 10, 24, 12, 0, 0, 0, ldn), nil,
			Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 25, 12, 0, 0, 0, ldn)}},
		{"elapsed hours across a DST change", "t > {NOW + 24::HOURS}", time.Date(2026, 10, 24, 12, 0, 0, 0, ldn), nil,
			Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 25, 11, 0, 0, 0, ldn)}},
		{"date across a DST change", "due > {TODAY + 1::DAYS}", time.Date(2026, 10, 24, 12, 0, 0, 0, ldn), nil,
			Value{Kind: ValueDate, Time: date(2026, 10, 25, ldn)}},
		{"order matters across a DST change", "t > {TODAY + 1::DAYS + 12::HOURS}", time.Date(2026, 10, 24, 12, 0, 0, 0, ldn), nil,
			Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 25, 11, 0, 0, 0, ldn)}},
		{"hours then days across a DST change", "t > {TODAY + 12::HOURS + 1::DAYS}", time.Date(2026, 10, 24, 12, 0, 0, 0, ldn), nil,
			Value{Kind: ValueTimestamp, Time: time.Date(2026, 10, 25, 12, 0, 0, 0, ldn)}},

		// Business days.
		{"business days over a weekend", "due < {TODAY + 3::BUSINESS_DAYS}", time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC), nil,
			Value{Kind: ValueDate, Time: date(2026, 11, 3, time.UTC)}},
		{"negative business days", "due < {TODAY - 1::BUSINESS_DAYS}", time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC), nil,
			Value{Kind: ValueDate, Time: date(2026, 10, 30, time.UTC)}},
		{"zero business days on a weekend", "due < {TODAY + 0::BUSINESS_DAYS}", time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC), nil,
			Value{Kind: ValueDate, Time: date(2026, 11, 2, time.UTC)}},
		{"zero business days on a weekday", "due < {TODAY + 0::BUSINESS_DAYS}", tuesday, nil,
			Value{Kind: ValueDate, Time: date(2026, 10, 27, time.UTC)}},
		{"business days keep the clock", "t < {NOW + 1::BUSINESS_DAYS}", time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC), nil,
			Value{Kind: ValueTimestamp, Time: time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)}},
		{"many business days", "due < {TODAY + 10::BUSINESS_DAYS}", tuesday, nil,
			Value{Kind: ValueDate, Time: date(2026, 11, 10, time.UTC)}},
		{"business days stay a date", "created_at < {TODAY + 1::DAYS + 1::BUSINESS_DAYS}", time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), nil,
			Value{Kind: ValueDate, Time: date(2026, 11, 2, time.UTC)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := planValue(t, tc.input, tc.now, tc.ws)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != tc.want.Kind || got.Number != tc.want.Number || !got.Time.Equal(tc.want.Time) {
				t.Fatalf("value = %v %q %v, want %v %q %v", got.Kind, got.Number, got.Time, tc.want.Kind, tc.want.Number, tc.want.Time)
			}
			if tc.want.Kind != ValueNumber {
				if got.Time.Location() != tc.now.Location() {
					t.Errorf("location = %v, want %v", got.Time.Location(), tc.now.Location())
				}
				if tc.want.Kind == ValueDate && got.Time.Format("15:04:05") != "00:00:00" {
					t.Errorf("date %v is not midnight", got.Time)
				}
			}
		})
	}
}

func TestDSTCalendarDayIs25Hours(t *testing.T) {
	ldn := london(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, ldn)
	got, err := planValue(t, "t > {NOW + 1::DAYS}", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := got.Time.Sub(now); d != 25*time.Hour {
		t.Errorf("elapsed = %v, want 25h", d)
	}
}

// pos returns the 1-based position of the nth occurrence (from 1) of sub.
func pos(input, sub string, nth int) int {
	at := -1
	for range nth {
		next := strings.Index(input[at+1:], sub)
		if next < 0 {
			panic(fmt.Sprintf("%q has no %d occurrences of %q", input, nth, sub))
		}
		at += 1 + next
	}
	return at + 1
}

func TestExpressionErrors(t *testing.T) {
	cases := []struct {
		name  string
		input string
		now   time.Time
		pos   int
		want  string
	}{
		{"missing reference time", "due = TODAY", time.Time{}, 7, "reference time"},
		{"missing reference time in braces", "due = {1::DAYS + TODAY}", time.Time{}, pos("due = {1::DAYS + TODAY}", "TODAY", 1), "TODAY needs a reference time"},
		{"bare duration", "due > {5::DAYS}", tuesday, 7, "duration"},
		{"bare duration sum", "due > {5::DAYS + 1::HOURS}", time.Time{}, 7, "duration"},
		{"non-terminating quotient", "price > {1 / 3}", time.Time{}, pos("price > {1 / 3}", "/", 1), "no finite decimal form"},
		{"non-terminating later cancelled", "price > {1 / 3 * 3}", time.Time{}, pos("price > {1 / 3 * 3}", "/", 1), "no finite decimal form"},
		{"division by zero", "price > {1 / (2 - 2)}", time.Time{}, pos("price > {1 / (2 - 2)}", "/", 1), "division by zero"},
		{"inexact duration division", "t > {NOW + (2::DAYS + 3::HOURS) / 2}", tuesday, pos("t > {NOW + (2::DAYS + 3::HOURS) / 2}", "/", 1), "not divisible"},
		{"duration division by zero", "t > {NOW + 2::DAYS / 0}", tuesday, pos("t > {NOW + 2::DAYS / 0}", "/", 1), "division by zero"},
		{"fractional divisor", "t > {NOW + 2::DAYS / 0.5}", tuesday, pos("t > {NOW + 2::DAYS / 0.5}", "/", 1), "not an integer"},
		{"fractional multiplier", "t > {NOW + 1.5 * 2::DAYS}", tuesday, pos("t > {NOW + 1.5 * 2::DAYS}", "*", 1), "not an integer"},
		{"amount too large", "t > {NOW + 10000001::DAYS}", tuesday, pos("t > {NOW + 10000001::DAYS}", "1", 1), "exceeds 10000000"},
		{"huge amount", "t > {NOW + 99999999999999999999999::DAYS}", tuesday, pos("t > {NOW + 99999999999999999999999::DAYS}", "9", 1), "exceeds 10000000"},
		{"scaled amount too large", "t > {NOW + 5000000::DAYS * 3}", tuesday, pos("t > {NOW + 5000000::DAYS * 3}", "*", 1), "exceeds 10000000"},
		{"time minus time", "t > {NOW - TODAY}", tuesday, pos("t > {NOW - TODAY}", "-", 1), "cannot apply -"},
		{"time plus time", "t > {NOW + TODAY}", tuesday, pos("t > {NOW + TODAY}", "+", 1), "cannot apply +"},
		{"duration minus time", "t > {1::DAYS - NOW}", tuesday, pos("t > {1::DAYS - NOW}", "-", 1), "cannot apply -"},
		{"time plus number", "t > {NOW + 1}", tuesday, pos("t > {NOW + 1}", "+", 1), "cannot apply +"},
		{"number plus duration", "t > {NOW + (1 + 1::DAYS)}", tuesday, pos("t > {NOW + (1 + 1::DAYS)}", "+", 2), "cannot apply +"},
		{"time times number", "t > {NOW * 2}", tuesday, pos("t > {NOW * 2}", "*", 1), "cannot apply *"},
		{"duration times duration", "t > {NOW + 1::DAYS * 1::DAYS}", tuesday, pos("t > {NOW + 1::DAYS * 1::DAYS}", "*", 1), "cannot apply *"},
		{"number divided by duration", "t > {NOW + 1 / 1::DAYS}", tuesday, pos("t > {NOW + 1 / 1::DAYS}", "/", 1), "cannot apply /"},
		{"negated time", "t > {-NOW}", tuesday, pos("t > {-NOW}", "-", 1), "cannot negate a timestamp"},
		{"type error before missing reference time", "t > {NOW - TODAY}", time.Time{}, pos("t > {NOW - TODAY}", "-", 1), "cannot apply -"},
		{"out of range", "due > {TODAY + 10000000::DAYS}", tuesday, 7, "years 1 to 9999"},
		{"out of range after a later step", "due > {TODAY + 2900000::DAYS + 2900000::DAYS}", tuesday, 7, "years 1 to 9999"},
		{"below year one", "due > {TODAY - 1::DAYS}", time.Date(1, 1, 1, 12, 0, 0, 0, time.UTC), 7, "years 1 to 9999"},
		{"keyword below year one", "due > START_OF_YEAR", time.Date(0, 6, 1, 0, 0, 0, 0, time.UTC), 7, "years 1 to 9999"},
		{"out of range business days", "due > {TODAY - 10000000::BUSINESS_DAYS}", tuesday, 7, "years 1 to 9999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planValue(t, tc.input, tc.now, nil)
			want := fmt.Sprintf("at position %d", tc.pos)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.HasSuffix(err.Error(), want) {
				t.Fatalf("err = %v, want %q %s", err, tc.want, want)
			}
		})
	}
}

func TestExpressionFieldCompatibility(t *testing.T) {
	cases := []struct {
		input string
		want  string // empty for success
	}{
		{"qty > {1 + 1}", ""},
		{"score > {1 + 1}", ""},
		{"price > {1 + 1}", ""},
		{"due > TODAY", ""},
		{"created_at > TODAY", ""},
		{"created_at > NOW", ""},
		{"created_at > {TODAY + 9::HOURS}", ""},
		{"age > TODAY", "cannot compare integer field \"age\" with a date at position 7"},
		{"age > NOW", "cannot compare integer field \"age\" with a timestamp at position 7"},
		{"due = NOW", "cannot compare date field \"due\" with a timestamp at position 7"},
		{"due = {TODAY + 1::HOURS}", "cannot compare date field \"due\" with a timestamp at position 7"},
		{"due = {1 + 1}", "cannot compare date field \"due\" with a number at position 7"},
		{"t = {1 + 1}", "cannot compare timestamp field \"t\" with a number at position 5"},
		{"name = {1 + 1}", "cannot compare text field \"name\" with a number at position 8"},
		{"name = TODAY", "cannot compare text field \"name\" with a date at position 8"},
		{"done = {1 + 1}", "cannot compare boolean field \"done\" with a number at position 8"},
		{"ref = NOW", "cannot compare uuid field \"ref\" with a timestamp at position 7"},
		{"state = TODAY", "cannot compare enum field \"state\" with a date at position 9"},
		{"days = TODAY", "array field \"days\" does not accept an expression value at position 8"},
		{"counts = {1 + 1}", "array field \"counts\" does not accept an expression value at position 10"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := mustTasks(t).Plan(KindPage, Request{Filter: parse(t, tc.input), Now: tuesday})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPlanValues(t *testing.T) {
	expr := parse(t, "qty > 3 AND due < {TODAY + 5::BUSINESS_DAYS} AND (price > {10 / 4} OR created_at >= START_OF_WEEK)")
	p, err := mustTasks(t).Plan(KindCount, Request{Filter: expr, Now: tuesday, WeekStart: weekday(time.Sunday)})
	if err != nil {
		t.Fatal(err)
	}
	preds := predicates(expr)
	if _, ok := p.Value(preds[0]); ok {
		t.Error("a number literal has a value")
	}
	want := []Value{
		{Kind: ValueDate, Time: date(2026, 11, 3, time.UTC)},
		{Kind: ValueNumber, Number: "2.5"},
		{Kind: ValueDate, Time: date(2026, 10, 25, time.UTC)},
	}
	for i, w := range want {
		if got, ok := p.Value(preds[i+1]); !ok || got != w {
			t.Errorf("value %d = %+v, %v; want %+v", i+1, got, ok, w)
		}
	}
	other := predicates(parse(t, "due < {TODAY + 5::BUSINESS_DAYS}"))[0]
	if _, ok := p.Value(other); ok {
		t.Error("a predicate of another tree has a value")
	}
	if _, ok := p.Value(nil); ok {
		t.Error("nil predicate has a value")
	}
	if _, ok := (&Plan{}).Value(preds[1]); ok {
		t.Error("an empty plan has a value")
	}

	// Plans are safe for concurrent reads.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if v, ok := p.Value(preds[2]); !ok || v.Number != "2.5" {
				t.Errorf("concurrent value = %+v, %v", v, ok)
			}
		})
	}
	wg.Wait()
}

func TestPlanRejectsInvalidWeekStart(t *testing.T) {
	_, err := mustTasks(t).Plan(KindPage, Request{Filter: parse(t, "due >= START_OF_WEEK"), Now: tuesday, WeekStart: weekday(9)})
	if err == nil || !strings.Contains(err.Error(), "week start") {
		t.Fatalf("err = %v", err)
	}
}

func TestBusinessDays(t *testing.T) {
	brute := func(wd time.Weekday, n int64) int64 {
		weekend := func(w time.Weekday) bool { return w == time.Saturday || w == time.Sunday }
		var delta int64
		if n == 0 {
			for weekend(wd) {
				wd, delta = (wd+1)%7, delta+1
			}
			return delta
		}
		step := int64(1)
		if n < 0 {
			step, n = -1, -n
		}
		for n > 0 {
			delta += step
			wd = time.Weekday((int64(wd) + 7 + step) % 7)
			if !weekend(wd) {
				n--
			}
		}
		return delta
	}
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		for n := int64(-30); n <= 30; n++ {
			if got, want := businessDays(wd, n), brute(wd, n); got != want {
				t.Errorf("businessDays(%v, %d) = %d, want %d", wd, n, got, want)
			}
		}
		for _, n := range []int64{1000003, -1000003} {
			if got, want := businessDays(wd, n), brute(wd, n); got != want {
				t.Errorf("businessDays(%v, %d) = %d, want %d", wd, n, got, want)
			}
		}
	}
}

func TestValueKindString(t *testing.T) {
	for k, want := range map[ValueKind]string{ValueNumber: "number", ValueDate: "date", ValueTimestamp: "timestamp", 0: "invalid", 9: "invalid"} {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", k, got, want)
		}
	}
}
