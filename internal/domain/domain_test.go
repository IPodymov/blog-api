package domain

import (
	"testing"
	"time"
)

func TestAccessRules(t *testing.T) {
	for _, tc := range []struct {
		a            Actor
		owner        int64
		status       string
		read, manage bool
	}{{Actor{}, 1, "published", true, false}, {Actor{}, 1, "draft", false, false}, {Actor{1, "user"}, 1, "draft", true, true}, {Actor{2, "user"}, 1, "draft", false, false}, {Actor{2, "admin"}, 1, "draft", true, true}} {
		if got := tc.a.CanRead(tc.owner, tc.status); got != tc.read {
			t.Errorf("read: %+v: %v", tc, got)
		}
		if got := tc.a.CanManage(tc.owner); got != tc.manage {
			t.Errorf("manage: %+v: %v", tc, got)
		}
	}
}
func TestPeriods(t *testing.T) {
	now := time.Date(2026, 10, 13, 23, 0, 0, 0, time.FixedZone("Moscow", 10800))
	for _, tc := range []struct {
		from, to string
		ok       bool
		days     int
	}{{"", "", true, 30}, {"2026-10-13", "2026-10-13", true, 1}, {"2026-10-12", "2026-10-13", true, 2}, {"2026-10-14", "2026-10-14", false, 0}, {"2026-10-13", "2026-10-12", false, 0}, {"2025-01-01", "2026-10-13", false, 0}, {"2026-2-01", "2026-10-13", false, 0}, {"2026-02-30", "2026-10-13", false, 0}} {
		p, e := ParsePeriod(tc.from, tc.to, now, 366)
		if (e == nil) != tc.ok {
			t.Errorf("%+v: %v", tc, e)
		}
		if e == nil && int(p.To.Sub(p.From)/(24*time.Hour))+1 != tc.days {
			t.Error("incorrect inclusive days")
		}
	}
}
