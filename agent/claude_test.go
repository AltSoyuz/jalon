package agent

import "testing"

// The CLI's JSON envelope carries the one number the kill criterion needs, and
// anything that is not an envelope is kept as the text it is: a run that died
// before printing one still leaves what it managed, and that is worth saving.
func TestUnwrap(t *testing.T) {
	for _, c := range []struct {
		name, in, out string
		cost          float64
	}{
		{"an envelope", `{"type":"result","result":"# Facts\n\nok","total_cost_usd":0.12}`, "# Facts\n\nok", 0.12},
		{"an envelope without a cost", `{"type":"result","result":"text"}`, "text", -1},
		{"plain text", "the CLI died\n", "the CLI died\n", -1},
		{"an empty output", "", "", -1},
		{"json that is not an envelope", `{"number":42}`, `{"number":42}`, -1},
	} {
		got := unwrap(c.in)
		if got.out != c.out || got.cost != c.cost {
			t.Errorf("%s: unwrap = %+v, want out %q cost %v", c.name, got, c.out, c.cost)
		}
	}
	if s := formatCost(sumCost(0.1, -1)); s != "unknown" {
		t.Errorf("a sum with an unknown must be unknown, got %s", s)
	}
	if s := formatCost(sumCost(0.1, 0.25)); s != "0.35 USD" {
		t.Errorf("sum = %s", s)
	}
}
