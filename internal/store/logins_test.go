package store_test

import (
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

var (
	day1 = time.Date(2027, 4, 1, 8, 0, 0, 0, time.UTC)
	day2 = day1.AddDate(0, 0, 1)
	day3 = day1.AddDate(0, 0, 2)
)

func TestLoginDays_CountsEachDayOnce(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordLogin(player.ID, day1)
	s.RecordLogin(player.ID, day1)
	s.RecordLogin(player.ID, day3)

	got, err := s.LoginDays(player.ID, day1, day3)

	updated, _ := s.Player(player.ID)
	if err != nil || got != 1 || updated.Logins != 3 {
		t.Fatalf("LoginDays() = %d, %v; logins %d", got, err, updated.Logins)
	}
}

func TestLastSeen_IsTheLatestLoginOrResultBeforeTheTime(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordLogin(player.ID, day1)
	s.RecordResult(player.ID, store.Result{Played: day2.Add(time.Hour)})
	s.RecordLogin(player.ID, day3)

	got, ok, err := s.LastSeen(player.ID, day3)

	if err != nil || !ok || !got.Equal(day2.Add(time.Hour)) {
		t.Fatalf("LastSeen() = %s, %v, %v", got, ok, err)
	}
}

func TestLastSeen_NeverSeen_ReportsFalse(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	if _, ok, err := s.LastSeen(player.ID, day3); ok || err != nil {
		t.Fatalf("LastSeen() = %v, %v", ok, err)
	}
}
