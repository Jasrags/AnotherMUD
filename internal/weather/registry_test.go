package weather_test

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/weather"
)

func TestRegistry_AddGet(t *testing.T) {
	r := weather.NewRegistry()
	z := &weather.Zone{ID: "temperate"}
	if err := r.Add(z); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := r.Get("temperate")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != z {
		t.Errorf("Get returned wrong pointer")
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
}

func TestRegistry_AddRejectsDuplicates(t *testing.T) {
	r := weather.NewRegistry()
	_ = r.Add(&weather.Zone{ID: "temperate"})
	err := r.Add(&weather.Zone{ID: "temperate"})
	if !errors.Is(err, weather.ErrDuplicateZone) {
		t.Errorf("duplicate Add returned %v, want ErrDuplicateZone", err)
	}
}

func TestRegistry_AddRejectsNilAndEmptyID(t *testing.T) {
	r := weather.NewRegistry()
	if err := r.Add(nil); err == nil {
		t.Error("Add(nil) returned no error")
	}
	if err := r.Add(&weather.Zone{}); err == nil {
		t.Error("Add(empty id) returned no error")
	}
}

func TestRegistry_GetMissingReturnsNotFound(t *testing.T) {
	r := weather.NewRegistry()
	_, err := r.Get("nope")
	if !errors.Is(err, weather.ErrZoneNotFound) {
		t.Errorf("Get(missing) = %v, want ErrZoneNotFound", err)
	}
}
