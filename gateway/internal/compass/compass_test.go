package compass

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionPinSurvivesConcurrentRequestsAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	cfg := Defaults()
	var router Router
	var calls atomic.Int32
	classify := func(context.Context) (string, string, error) {
		calls.Add(1)
		return "mid", "Ordinary implementation", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := router.Pick(context.Background(), path, SessionKey("codex", "account", "thread"), cfg, false, classify)
			if err != nil || d.Model != cfg.Mid {
				t.Errorf("decision %+v, %v", d, err)
			}
		}()
	}
	wg.Wait()
	var restarted Router
	cfg.Mid = "another/model"
	d, err := restarted.Pick(context.Background(), path, SessionKey("codex", "account", "thread"), cfg, false, classify)
	if err != nil || d.Model != Defaults().Mid || calls.Load() != 1 {
		t.Fatalf("session changed: %+v, calls %d, error %v", d, calls.Load(), err)
	}
	_, err = restarted.Pick(context.Background(), path, SessionKey("codex", "account", "child-thread"), cfg, true, classify)
	if err != nil || calls.Load() != 2 {
		t.Fatalf("child did not receive separate classification: %v", err)
	}
}
func TestClassifierFailurePinsDefault(t *testing.T) {
	for _, fail := range []bool{true, false} {
		path := filepath.Join(t.TempDir(), "compass.json")
		var router Router
		d, err := router.Pick(context.Background(), path, SessionKey("c", "a", "t"), Defaults(), false, func(context.Context) (string, string, error) {
			if fail {
				return "", "", fmt.Errorf("timeout")
			}
			return "arbitrary-model", "bad output", nil
		})
		if err != nil || d.Tier != "default" || d.Model != Defaults().Default {
			t.Fatalf("failure decision: %+v %v", d, err)
		}
	}
}
func TestMissingIdentityDoesNotClassify(t *testing.T) {
	var router Router
	d, err := router.Pick(context.Background(), "unused", "", Defaults(), true, func(context.Context) (string, string, error) {
		t.Fatal("classified without identity")
		return "", "", nil
	})
	if err != nil || d.Model != Defaults().Default {
		t.Fatalf("fallback %+v %v", d, err)
	}
}

func TestRecentDecisionsHaveStablePrivateIdentityAndNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	for i := 0; i < 23; i++ {
		key := SessionKey("codex", "private-account", fmt.Sprintf("private-thread-%d", i))
		d := Decision{Model: Defaults().Low, Tier: "low", CreatedAt: time.Unix(int64(i), 0)}
		if err := saveDecision(path, key, d); err != nil {
			t.Fatal(err)
		}
	}
	recent := Recent(path)
	if len(recent) != 20 {
		t.Fatalf("got %d decisions", len(recent))
	}
	for i, d := range recent {
		want := SessionKey("codex", "private-account", fmt.Sprintf("private-thread-%d", 22-i))
		if d.ID != want {
			t.Fatalf("wrong order or ID: %+v", d)
		}
	}
	again := Recent(path)
	if again[0].ID != recent[0].ID {
		t.Fatal("identity changed on refresh")
	}
}

func TestComparisonDefaultsToHighAndDoesNotAllowRecursiveModel(t *testing.T) {
	c := Defaults()
	if c.ComparisonModel() != c.High {
		t.Fatal("wrong initial comparison")
	}
	c.Comparison = "example/comparison"
	if c.Validate() != nil || c.ComparisonModel() != c.Comparison {
		t.Fatal("explicit comparison rejected")
	}
	c.Comparison = LegacyModel
	if c.Validate() == nil {
		t.Fatal("recursive comparison accepted")
	}
}
