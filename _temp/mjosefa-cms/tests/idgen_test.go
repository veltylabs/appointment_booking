package tests

import "webtyp.com/fmt"

// testIDGen is the composition-root double for model.IDGenerator — mjosefa-cms
// never constructs its own generator (same rule as form.New and crudview.New),
// so every test injects one instead of passing nil.
type testIDGen struct{ n int }

func (g *testIDGen) NewID() string {
	g.n++
	return "test-id-" + fmt.Convert(g.n).String()
}
