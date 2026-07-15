package installer

import (
	"reflect"
	"testing"
)

// noEnv is a getenv that reports every var as unset.
func noEnv(string) (string, bool) { return "", false }

func envFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestMergeInstallEnv_PreservesWhenNotResupplied(t *testing.T) {
	// The regression: a bare reinstall (no flags, no shell env) must carry the
	// previously-baked onboarding vars over instead of wiping them.
	prev := map[string]string{
		"SWARMERY_ONBOARD_ROOTS":  "/home/dev/projects",
		"SWARMERY_WORKSPACE_ROOT": "/home/dev/swarmery-workspace",
	}
	env, preserved := mergeInstallEnv(prev, map[string]bool{}, map[string]string{}, noEnv)

	want := []EnvVar{
		{Key: "SWARMERY_ONBOARD_ROOTS", Value: "/home/dev/projects"},
		{Key: "SWARMERY_WORKSPACE_ROOT", Value: "/home/dev/swarmery-workspace"},
	}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("env = %+v, want %+v", env, want)
	}
	if len(preserved) != 2 {
		t.Errorf("preserved = %v, want both keys", preserved)
	}
}

func TestMergeInstallEnv_ExplicitFlagWins(t *testing.T) {
	prev := map[string]string{"SWARMERY_ONBOARD_ROOTS": "/old"}
	env, _ := mergeInstallEnv(prev,
		map[string]bool{"onboard-roots": true},
		map[string]string{"onboard-roots": "/new,/other"},
		noEnv)
	if len(env) != 1 || env[0].Value != "/new,/other" {
		t.Errorf("explicit flag ignored: %+v", env)
	}
}

func TestMergeInstallEnv_ExplicitEmptyClears(t *testing.T) {
	prev := map[string]string{"SWARMERY_ONBOARD_ROOTS": "/old"}
	env, preserved := mergeInstallEnv(prev,
		map[string]bool{"onboard-roots": true},
		map[string]string{"onboard-roots": ""},
		noEnv)
	if len(env) != 0 {
		t.Errorf("explicit empty should clear, got %+v", env)
	}
	if len(preserved) != 0 {
		t.Errorf("nothing should be preserved when explicitly cleared, got %v", preserved)
	}
}

func TestMergeInstallEnv_ShellEnvBeatsPrev(t *testing.T) {
	prev := map[string]string{"SWARMERY_ONBOARD_ROOTS": "/old"}
	env, preserved := mergeInstallEnv(prev, map[string]bool{}, map[string]string{},
		envFrom(map[string]string{"SWARMERY_ONBOARD_ROOTS": "/from-shell"}))
	if len(env) != 1 || env[0].Value != "/from-shell" {
		t.Errorf("shell env should win over prev: %+v", env)
	}
	if len(preserved) != 0 {
		t.Errorf("shell env is not a preservation: %v", preserved)
	}
}

func TestResolveInstallPort(t *testing.T) {
	if got := resolveInstallPort(nil, true, 9000); got != 9000 {
		t.Errorf("explicit port = %d, want 9000", got)
	}
	if got := resolveInstallPort(map[string]string{"SWARMERY_PORT": "8123"}, false, -1); got != 8123 {
		t.Errorf("preserved port = %d, want 8123", got)
	}
	if got := resolveInstallPort(nil, false, -1); got != 0 {
		t.Errorf("default port = %d, want 0", got)
	}
}
