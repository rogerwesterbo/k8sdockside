package appconfig

import "testing"

func TestWindowNeverRecorded(t *testing.T) {
	store := openIn(t)

	if got := store.Get().Window; got != (Window{}) {
		t.Errorf("window = %+v, want the zero value", got)
	}
}

func TestWindowRoundTrip(t *testing.T) {
	path := tempSettings(t)

	store, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	// A screen to the left of the primary one gives a negative X.
	want := Window{X: -1600, Y: 40, Width: 1500, Height: 950, Maximised: true}
	if _, err := store.SetWindow(want); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}

	reopened, err := openAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get().Window; got != want {
		t.Errorf("window = %+v, want %+v", got, want)
	}
}
