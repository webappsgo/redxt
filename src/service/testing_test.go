package service

import "strings"

// fakeIDLookup is a deterministic, in-memory IDLookup for tests. No real
// /etc/passwd or /etc/group lookup ever happens.
type fakeIDLookup struct {
	takenUIDs map[int]bool
	takenGIDs map[int]bool
}

func newFakeIDLookup() *fakeIDLookup {
	return &fakeIDLookup{takenUIDs: map[int]bool{}, takenGIDs: map[int]bool{}}
}

func (f *fakeIDLookup) LookupUID(id int) bool { return f.takenUIDs[id] }
func (f *fakeIDLookup) LookupGID(id int) bool { return f.takenGIDs[id] }

// fakeFileLookup is an in-memory FileLookup for tests. No real os.Stat of
// any host path ever happens.
type fakeFileLookup struct {
	present map[string]bool
}

func newFakeFileLookup(paths ...string) *fakeFileLookup {
	f := &fakeFileLookup{present: map[string]bool{}}
	for _, p := range paths {
		f.present[p] = true
	}
	return f
}

func (f *fakeFileLookup) Exists(path string) bool { return f.present[path] }

// fakePathLookup is an in-memory PathLookup for tests. No real PATH probe
// of any host binary ever happens.
type fakePathLookup struct {
	found map[string]bool
}

func newFakePathLookup(names ...string) *fakePathLookup {
	f := &fakePathLookup{found: map[string]bool{}}
	for _, n := range names {
		f.found[n] = true
	}
	return f
}

func (f *fakePathLookup) LookPath(name string) (string, error) {
	if f.found[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errNotFound
}

var errNotFound = notFoundError("not found")

type notFoundError string

func (e notFoundError) Error() string { return string(e) }

// fakeRunner records every command it is asked to run instead of actually
// invoking os/exec, so no test ever creates a user, writes to /etc, or
// invokes systemctl/rc-service/launchctl/etc.
type fakeRunner struct {
	calls    [][]string
	failOn   map[string]bool
	outputs  map[string][]byte
	runErr   error
	runNever bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{failOn: map[string]bool{}, outputs: map[string][]byte{}}
}

func (f *fakeRunner) key(name string, args ...string) string {
	return name + " " + strings.Join(args, " ")
}

func (f *fakeRunner) Run(name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failOn[f.key(name, args...)] {
		return notFoundError("fake failure: " + f.key(name, args...))
	}
	return nil
}

func (f *fakeRunner) Output(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failOn[f.key(name, args...)] {
		return nil, notFoundError("fake failure: " + f.key(name, args...))
	}
	return f.outputs[f.key(name, args...)], nil
}

// noopLogger discards everything logged - used so tests can pass a
// non-nil Logger without asserting on log content.
type noopLogger struct{}

func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}
