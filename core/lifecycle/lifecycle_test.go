package lifecycle

import "testing"

type testCloser struct {
	closed int
}

func (c *testCloser) Close() { c.closed++ }

func TestRegisterSkippedInTests(t *testing.T) {
	c := &testCloser{}
	Register(c)
	CloseAll()
	if c.closed != 0 {
		t.Fatalf("Register during go test must not retain closers, closed=%d", c.closed)
	}
}

func TestCloseAllReverseOrder(t *testing.T) {
	var order []int
	register(&fnCloser{fn: func() { order = append(order, 1) }}, false)
	register(&fnCloser{fn: func() { order = append(order, 2) }}, false)
	CloseAll()
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("CloseAll order=%v, want reverse registration", order)
	}
	CloseAll()
	if len(order) != 2 {
		t.Fatal("second CloseAll must be a no-op")
	}
}

type fnCloser struct{ fn func() }

func (c *fnCloser) Close() { c.fn() }

func TestUnregisterReleasesClosedResources(t *testing.T) {
	c := &testCloser{}
	unregister := register(c, false)
	unregister()
	unregister()
	CloseAll()
	if c.closed != 0 {
		t.Fatal("unregistered resource retained")
	}
}

func TestCloserCanUnregisterDuringShutdown(t *testing.T) {
	var unregister func()
	c := &fnCloser{fn: func() { unregister() }}
	unregister = register(c, false)
	CloseAll()
	unregister()
}
